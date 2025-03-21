package websocket

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"openchamp/server/internal/util"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
)

type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func handlePacket(client *Client, message_string string, log *logrus.Logger) {
	// Try to parse the message as JSON
	var message Message
	if err := json.Unmarshal([]byte(message_string), &message); err != nil {
		// If the message is not JSON, just log it as a string
		log.WithFields(logrus.Fields{
			"client_id": client.id,
			"message":   message_string,
		}).Info("Received String as Message")
	}

	switch message.Type {
	case "login":
		client.handleAuthentication(message)
	case "register":
		client.handleRegistration(message)
	case "token_auth":
		// Handle token authentication request
		// Token-based authentication
		var tokenAuth struct {
			Token string `json:"token"`
		}
		// Extract client's real IP address from connection
		clientIP := client.getClientIP()

		// Validate token against database
		username, valid, err := client.validateToken(tokenAuth.Token, clientIP)
		if err != nil || !valid {
			client.sendError("token_error", "Invalid or expired token")
			return
		}

		// Authentication successful
		client.completeAuthentication(username, tokenAuth.Token)

	}

}

func (client *Client) getClientIP() string {
	// Extract IP from WebSocket connection
	// First, get the remote address from the connection
	remoteAddr := client.conn.RemoteAddr().String()

	// Extract just the IP part (remove port)
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// Fallback to full address if we can't split it
		return remoteAddr
	}

	return ip
}
func (client *Client) handleAuthentication(msg Message) {
	switch msg.Type {
	case "login":
		// Username/password authentication
		var credentials struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}

		if err := json.Unmarshal(msg.Payload, &credentials); err != nil {
			log.Printf("Error parsing login credentials: %v", err)
			client.sendError("","Invalid login format")
			return
		}

		// Validate credentials against database
		authenticated, token, err := client.validateCredentials(credentials.Username, credentials.Password)
		if err != nil || !authenticated {
			client.sendAuthError("Invalid username or password")
			return
		}

		// Authentication successful
		client.completeAuthentication(credentials.Username, token)

	case "token_auth":
		// Token-based authentication
		var tokenAuth struct {
			Token string `json:"token"`
		}

		if err := json.Unmarshal(msg.Payload, &tokenAuth); err != nil {
			log.Printf("Error parsing token auth: %v", err)
			client.sendAuthError("Invalid token format")
			return
		}

		// Extract client's real IP address from connection
		clientIP := client.getClientIP()

		// Validate token against database
		username, valid, err := client.validateToken(tokenAuth.Token, clientIP)
		if err != nil || !valid {
			client.sendAuthError("Invalid or expired token")
			return
		}

		// Authentication successful
		client.completeAuthentication(username, tokenAuth.Token)
	}
}

func (client *Client) handleRegistration(msg Message) {
	var registration struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	if err := json.Unmarshal(msg.Payload, &registration); err != nil {
		log.Printf("Error parsing registration data: %v", err)
		client.sendError("registration_error", "Invalid registration format")
		return
	}

	// Validate input
	if len(registration.Username) < 3 {
		client.sendError("registration_error", "Username must be at least 3 characters")
		return
	}

	if len(registration.Password) < 6 {
		client.sendError("registration_error", "Password must be at least 6 characters")
		return
	}

	if registration.Email != "" && !util.IsValidEmail(registration.Email) {
		client.sendError("registration_error", "Invalid email format")
		return
	}

	// Check if username already exists
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var exists bool
	err := client.dbPool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)",
		registration.Username).Scan(&exists)

	if err != nil {
		log.Printf("Database error during registration: %v", err)
		client.sendError("registration_error", "Registration failed due to a server error")
		return
	}

	if exists {
		client.sendError("registration_error", "Username already exists")
		return
	}

	// Check if email already exists (if provided)
	if registration.Email != "" {
		err = client.dbPool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)",
			registration.Email).Scan(&exists)

		if err != nil {
			log.Printf("Database error during registration: %v", err)
			client.sendError("registration_error", "Registration failed due to a server error")
			return
		}

		if exists {
			client.sendError("registration_error", "Email already registered")
			return
		}
	}

	// Hash the password using bcrypt
	password, err := bcrypt.GenerateFromPassword([]byte(registration.Password), 12)
	if err != nil {
		log.Printf("Error hashing password: %v", err)
		client.sendError("registration_error", "Registration failed due to a server error")
		return
	}
	passwordHash := string(password)

	// Insert user into database
	_, err = client.dbPool.Exec(ctx,
		"INSERT INTO users (username, password_hash, email) VALUES ($1, $2, $3)",
		registration.Username, passwordHash, registration.Email)

	if err != nil {
		log.Printf("Error inserting new user: %v", err)
		client.sendError("registration_error", "Registration failed due to a server error")
		return
	}

	// Generate token for automatic login
	token := uuid.New().String()

	// Get the user ID for the token
	var userID int
	err = client.dbPool.QueryRow(ctx,
		"SELECT id FROM users WHERE username = $1",
		registration.Username).Scan(&userID)

	if err != nil {
		log.Printf("Error retrieving new user ID: %v", err)
		// Registration was successful, but auto-login failed
		client.sendRegistrationSuccess(false, "", "")
		return
	}

	// Get client's real IP
	clientIP := client.getClientIP()

	// Store token with IP
	_, err = client.dbPool.Exec(ctx,
		"INSERT INTO auth_tokens (user_id, token, ip_address, created_at, expires_at) VALUES ($1, $2, $3, NOW(), NOW() + INTERVAL '7 days')",
		userID, token, clientIP)

	if err != nil {
		log.Printf("Error creating token for new user: %v", err)
		// Registration was successful, but auto-login failed
		client.sendRegistrationSuccess(false, "", "")
		return
	}

	// Registration and auto-login successful
	client.username = registration.Username
	client.authenticated = true
	client.authToken = token

	// Send success response with auto-login token
	client.sendRegistrationSuccess(true, registration.Username, token)

	log.Printf("New user registered and authenticated: %s", registration.Username)
}

func (client *Client) validateToken(token, clientIP string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		username string
		storedIP sql.NullString
		tokenID  int
	)

	// Query the database to validate the token
	err := client.dbPool.QueryRow(ctx,
		`SELECT t.id, u.username, t.ip_address
		FROM auth_tokens t
		JOIN users u ON t.user_id = u.id
		WHERE t.token = $1 
		AND t.expires_at > NOW()`,
		token).Scan(&tokenID, &username, &storedIP)

	if err != nil {
		if err == pgx.ErrNoRows {
			return "", false, nil // Token not found or expired
		}
		return "", false, err // Database error
	}

	// Check IP restrictions if a previous IP is stored
	if storedIP.Valid && storedIP.String != "" {
		// If this is a different IP than previously used with this token,
		// we can either reject it or implement additional security checks
		if storedIP.String != clientIP {
			log.Printf("Warning: Token used from new IP. Original: %s, Current: %s",
				storedIP.String, clientIP)

			// Depending on security requirements, you might want to:
			// 1. Reject the attempt (uncomment the next line)
			// return "", false, nil

			// 2. Allow it but track the new IP
			// 3. Require additional verification
			// 4. Rate limit new IP logins
		}
	}

	// Update the token's last_used_at timestamp and IP
	_, err = client.dbPool.Exec(ctx,
		`UPDATE auth_tokens 
		SET last_used_at = NOW(), 
		    ip_address = $1
		WHERE id = $2`,
		clientIP, tokenID)

	if err != nil {
		log.Printf("Error updating token usage: %v", err)
		// Non-critical error, we can continue
	}

	return username, true, nil
}
func (client *Client) sendError(category string, message string) {
	response := map[string]interface{}{
		"type": "error",
		"payload": map[string]interface{}{
			"subtype": category,
			"message": message,
		},
	}
	responseJSON, _ := json.Marshal(response)
	client.send <- responseJSON
}
func (client *Client) completeAuthentication(username string, token string) {
	client.authenticated = true
	client.username = username
	client.authToken = token

	// Send successful authentication response
	response := map[string]interface{}{
		"type": "auth_success",
		"payload": map[string]interface{}{
			"username": username,
			"token":    token,
		},
	}
	responseJSON, _ := json.Marshal(response)
	client.send <- responseJSON

	log.Printf("Client authenticated: %s as %s", client.id, username)
}

func (client *Client) sendRegistrationSuccess(autoLogin bool, username, token string) {
	payload := map[string]interface{}{
		"success":   true,
		"message":   "Registration successful",
		"autoLogin": autoLogin,
	}

	// Include login info if auto-login succeeded
	if autoLogin {
		payload["username"] = username
		payload["token"] = token
	}

	response := map[string]interface{}{
		"type":    "register_success",
		"payload": payload,
	}

	responseJSON, _ := json.Marshal(response)
	client.send <- responseJSON
}
