package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionDuration = 24 * time.Hour

var sessions = make(map[string]session)

type session struct {
	username string
	expiry   time.Time
}

func (s session) isExpired() bool {
	return s.expiry.Before(time.Now())
}

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	InvitationCode string `json:"invitationCode"`
}

func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func checkPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func signup(w http.ResponseWriter, r *http.Request) {
	var creds Credentials
	err := json.NewDecoder(r.Body).Decode(&creds)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Check if invitation code is valid
	var used bool
	err = db.QueryRow("SELECT used FROM invitation_codes WHERE code = ?", creds.InvitationCode).Scan(&used)
	if err != nil {
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if used {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	hashedPassword, err := hashPassword(creds.Password)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	stmt, err := db.Prepare("INSERT INTO users (username, password_hash) VALUES (?, ?)")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	_, err = stmt.Exec(creds.Username, hashedPassword)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Mark invitation code as used
	stmt, err = db.Prepare("UPDATE invitation_codes SET used = true WHERE code = ?")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	_, err = stmt.Exec(creds.InvitationCode)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func signin(w http.ResponseWriter, r *http.Request) {
	var creds Credentials
	err := json.NewDecoder(r.Body).Decode(&creds)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var hashedPassword string
	err = db.QueryRow("SELECT password_hash FROM users WHERE username = ?", creds.Username).Scan(&hashedPassword)
	if err != nil {
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !checkPasswordHash(creds.Password, hashedPassword) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	sessionToken := base64.StdEncoding.EncodeToString(make([]byte, 32))
	_, err = rand.Read([]byte(sessionToken))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	expiresAt := time.Now().Add(sessionDuration)
	sessions[sessionToken] = session{username: creds.Username, expiry: expiresAt}

	http.SetCookie(w, &http.Cookie{
		Name:    "session_token",
		Value:   sessionToken,
		Expires: expiresAt,
	})
}

func logout(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("session_token")
	if err != nil {
		if err == http.ErrNoCookie {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	sessionToken := c.Value

delete(sessions, sessionToken)

	http.SetCookie(w, &http.Cookie{
		Name:    "session_token",
		Value:   "",
		Expires: time.Now(),
	})
}

func checkAuth(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("session_token")
	if err != nil {
		if err == http.ErrNoCookie {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	sessionToken := c.Value
	userSession, exists := sessions[sessionToken]
	if !exists {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if userSession.isExpired() {
		delete(sessions, sessionToken)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func generateInviteCode(w http.ResponseWriter, r *http.Request) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	code := base64.URLEncoding.EncodeToString(b)

	stmt, err := db.Prepare("INSERT INTO invitation_codes (code, used) VALUES (?, ?)")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	_, err = stmt.Exec(code, false)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"code": code})
}
