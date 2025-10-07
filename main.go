package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3" // go get github.com/mattn/go-sqlite3
)

var db *sql.DB

type Post struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Date    string `json:"date"`
}

type Project struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

type SysInfo struct {
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	KernelVersion string `json:"kernelVersion"`
	CPUName       string `json:"cpuName"`
	NumCPU        int    `json:"numCpu"`
	MemTotal      string `json:"memTotal"`
	MemUsed       string `json:"memUsed"`
	LoadAvg       string `json:"loadAvg"`
}

func getKernelVersion() string {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return "N/A"
	}
	return strings.Fields(string(data))[2]
}

func getCPUInfo() (string, int) {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "N/A", runtime.NumCPU()
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "model name") {
			return strings.TrimSpace(strings.Split(line, ":")[1]), runtime.NumCPU()
		}
	}
	return "N/A", runtime.NumCPU()
}

func getMemInfo() (string, string) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return "N/A", "N/A"
	}
	lines := strings.Split(string(data), "\n")
	var memTotal, memAvailable uint64
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			memTotal = val
		case "MemAvailable:":
			memAvailable = val
		}
	}
	if memTotal == 0 {
		return "N/A", "N/A"
	}
	memUsed := memTotal - memAvailable
	return fmt.Sprintf("%.2f GB", float64(memTotal)/1024/1024), fmt.Sprintf("%.2f GB", float64(memUsed)/1024/1024)
}

func getLoadAvg() string {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return "N/A"
	}
	return strings.Fields(string(data))[0]
}

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", "./nrv.dev.db")
	if err != nil {
		log.Fatal(err)
	}

	createTables := `
	CREATE TABLE IF NOT EXISTS visitors (id INTEGER PRIMARY KEY, ip_address TEXT UNIQUE, timestamp DATETIME);
	CREATE TABLE IF NOT EXISTS posts (id INTEGER PRIMARY KEY, slug TEXT, title TEXT, content TEXT, date TEXT);
	CREATE TABLE IF NOT EXISTS projects (id INTEGER PRIMARY KEY, name TEXT, description TEXT, url TEXT);
	CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, username TEXT UNIQUE, password_hash TEXT);
	`
	_, err = db.Exec(createTables)
	if err != nil {
		log.Fatal(err)
	}

	// Migrate initial content
	migrateContent()
}

func migrateContent() {
	// Check if posts exist
	var count int
	db.QueryRow("SELECT COUNT(*) FROM posts").Scan(&count)
	if count == 0 {
		log.Println("Migrating blog posts...")
		stmt, _ := db.Prepare("INSERT INTO posts (slug, title, content, date) VALUES (?, ?, ?, ?)")
		stmt.Exec("gemini-cli", "Gemini CLI: Building a Golang-Powered AI Tool", `
                    <div class="space-y-4">
                        <h2 class="text-xl font-bold text-yellow-300">Project: Gemini CLI</h2>
                        <p class="text-gray-400">Date: 2025-10-05</p>
                        <p>This project is a command-line interface built with Golang that interacts with Google's Gemini API. The goal was to create a lightweight, fast, and extensible tool for developers and power-users to leverage generative AI directly from their terminal.</p>
                        <h3 class="font-bold text-cyan-300">Core Features:</h3>
                        <ul class="list-disc list-inside pl-4 space-y-1">
                            <li>Interactive chat mode.</li>
                            <li>Direct query execution with streaming output.</li>
                            <li>Configuration via YAML for easy API key management.</li>
                        </ul>
                        <p>The choice of Golang was driven by its performance, concurrency model, and suitability for creating self-contained binaries, making distribution and installation trivial across different operating systems.</p>
                    </div>
                `, "2025-10-05")
		stmt.Close()
	}

	// Check if projects exist
	db.QueryRow("SELECT COUNT(*) FROM projects").Scan(&count)
	if count == 0 {
		log.Println("Migrating projects...")
		stmt, _ := db.Prepare("INSERT INTO projects (name, description, url) VALUES (?, ?, ?)")
		stmt.Exec("github/nullrootvector", "", "https://github.com/nullrootvector")
		stmt.Close()
	}
}

func visitorMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		stmt, _ := db.Prepare("INSERT OR IGNORE INTO visitors (ip_address, timestamp) VALUES (?, ?)")
		stmt.Exec(ip, time.Now())
		stmt.Close()
		next.ServeHTTP(w, r)
	})
}

func main() {
	port := flag.String("port", "8443", "port to listen on")
	flag.Parse()

	initDB()
	defer db.Close()

	fs := http.FileServer(http.Dir("."))
	http.Handle("/", visitorMiddleware(fs))

	http.HandleFunc("/api/sysinfo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		cpuName, numCPU := getCPUInfo()
		memTotal, memUsed := getMemInfo()
		info := SysInfo{
			OS:            runtime.GOOS,
			Arch:          runtime.GOARCH,
			KernelVersion: getKernelVersion(),
			CPUName:       cpuName,
			NumCPU:        numCPU,
			MemTotal:      memTotal,
			MemUsed:       memUsed,
			LoadAvg:       getLoadAvg(),
		}
		json.NewEncoder(w).Encode(info)
	})

	http.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		var count int
		db.QueryRow("SELECT COUNT(*) FROM visitors").Scan(&count)
		json.NewEncoder(w).Encode(map[string]int{"visitors": count})
	})

	http.HandleFunc("/api/posts", func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, slug, title, content, date FROM posts")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		posts := make(map[string]Post)
		for rows.Next() {
			var p Post
			var slug string
			err := rows.Scan(&p.ID, &slug, &p.Title, &p.Content, &p.Date)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			posts[slug] = p
		}
		json.NewEncoder(w).Encode(posts)
	})

	http.HandleFunc("/api/projects", func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, name, description, url FROM projects")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var projects []Project
		for rows.Next() {
			var p Project
			err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.URL)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			projects = append(projects, p)
		}
		json.NewEncoder(w).Encode(projects)
	})

	log.Printf("Starting server on https://localhost:%s", *port)
	if err := http.ListenAndServeTLS(":"+*port, "cert.pem", "key.pem", nil); err != nil {
		log.Fatal(err)
	}
}
