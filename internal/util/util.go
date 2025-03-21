package util

import (
	"fmt"
	"regexp"
)

func ConsoleTitle() {
	// Clear the console
	fmt.Print("\033[H\033[2J")
	// Print cool customized logo
	fmt.Println(`||=========================================================================||`)
	fmt.Println(`|| ________                       _________ .__                            ||`)
	fmt.Println(`|| \_____  \ ______   ____   ____ \_   ___ \|  |__ _____    _____ ______   ||`)
	fmt.Println(`||  /   |   \\____ \_/ __ \ /    \/    \  \/|  |  \\__  \  /     \\____ \  ||`)
	fmt.Println(`|| /    |    \  |_> >  ___/|   |  \     \___|   Y  \/ __ \|  Y Y  \  |_> > ||`)
	fmt.Println(`|| \_______  /   __/ \___  >___|  /\______  /___|  (____  /__|_|  /   __/  ||`)
	fmt.Println(`||         \/|__|        \/     \/        \/     \/     \/      \/|__|     ||`)
	fmt.Println(`|| ========================================================================||`)
}

func IsValidEmail(email string) bool {
	emailRegex := regexp.MustCompile(`^[A-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}$`)
	return emailRegex.MatchString(email)
}
