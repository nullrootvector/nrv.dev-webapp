package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"
)

func runCLI() {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		switch input {
		case "generate-invite":
			generateInviteCodeCLI()
		case "read-inquiries":
			readInquiriesCLI()
		case "exit":
			return
		default:
			fmt.Printf("Unknown command: %s\n", input)
		}
	}
}

func readInquiriesCLI() {
	rows, err := db.Query("SELECT name, email, message, ip_address, timestamp FROM inquiries")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, email, message, ip_address, timestamp string
		err := rows.Scan(&name, &email, &message, &ip_address, &timestamp)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("\nName: %s\nEmail: %s\nMessage: %s\nIP Address: %s\nTimestamp: %s\n", name, email, message, ip_address, timestamp)
	}
}

func generateInviteCodeCLI() {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		log.Fatal(err)
	}
	code := base64.URLEncoding.EncodeToString(b)

	stmt, err := db.Prepare("INSERT INTO invitation_codes (code, used) VALUES (?, ?)")
	if err != nil {
		log.Fatal(err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(code, false)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Invitation code: %s\n", code)
}