package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv" // Импортируем godotenv
)

var tpl = template.Must(template.ParseFiles("index.html"))

var apiKey *string

type Search struct {
	SearchKey    string
	CurrentPage  int
	TotalPages   int
	PreviousPage int
	NextPage     int
	Results      Results
}

// IsLastPage проверяет, является ли текущая страница последней.
func (s *Search) IsLastPage() bool {
	return s.CurrentPage >= s.TotalPages
}

// HasPreviousPage проверяет, есть ли предыдущая страница.
func (s *Search) HasPreviousPage() bool {
	return s.CurrentPage > 1
}

func (s *Search) HasNextPage() bool { // <-- Важно: HasNextPage с большой буквы
	return s.CurrentPage < s.TotalPages
}

type Source struct {
	ID   interface{} `json:"id"`
	Name string      `json:"name"`
}

type Article struct {
	Source      Source    `json:"source"`
	Author      string    `json:"author"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	URLToImage  string    `json:"urlToImage"`
	PublishedAt time.Time `json:"publishedAt"`
	Content     string    `json:"content"`
}

// FormatPublishedDate форматирует дату публикации статьи.
func (a *Article) FormatPublishedDate() string {
	year, month, day := a.PublishedAt.Date()
	return fmt.Sprintf("%v %d, %d", month, day, year)
}

type Results struct {
	Status       string    `json:"status"`
	TotalResults int       `json:"totalResults"`
	Articles     []Article `json:"articles"`
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	err := tpl.Execute(w, nil)
	if err != nil {
		log.Printf("Error executing template: %v", err) // Added logging
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
	}
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	// Parse URL and get parameters
	u, err := url.Parse(r.URL.String())
	if err != nil {
		log.Printf("Error parsing URL: %v", err) // Added logging
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	params := u.Query()
	searchKey := params.Get("q")
	pageStr := params.Get("page") // Get page as string
	pageSize := 20                // Set page size

	// Convert page string to integer
	page := 1 // Default page
	if pageStr != "" {
		p, err := strconv.Atoi(pageStr)
		if err != nil {
			log.Printf("Error converting page to integer: %v", err)
			http.Error(w, "Invalid page number", http.StatusBadRequest)
			return
		}
		page = p
	}

	// Create a Search struct
	search := &Search{
		SearchKey:   searchKey,
		CurrentPage: page,
	}

	// Call NewsAPI
	results, err := getNews(searchKey, pageSize, page)
	if err != nil {
		log.Printf("Error getting news: %v", err) // Added logging
		http.Error(w, "Failed to get news", http.StatusInternalServerError)
		return
	}
	if results.TotalResults == 0 {
		search.Results.TotalResults = 0
	}
	search.Results = results
	search.CurrentPage = page

	totalPages := 1
	if results.TotalResults > 0 {
		totalPages = int(math.Ceil(float64(results.TotalResults) / float64(pageSize)))
	}

	if search.CurrentPage > 1 {
		search.PreviousPage = search.CurrentPage - 1
	} else {
		search.PreviousPage = 1 // Или 0, в зависимости от вашей логики
	}

	if search.CurrentPage < search.TotalPages {
		search.NextPage = search.CurrentPage + 1
	} else {
		search.NextPage = search.TotalPages
	}

	search.TotalPages = totalPages

	log.Printf("search.Results.TotalResults = %v (type %T)", search.Results.TotalResults, search.Results.TotalResults) // Логирование для проверки
	err = tpl.Execute(w, search)
	if err != nil {
		log.Printf("Error executing template: %v", err) // Added logging
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
	}
}

// getNews делает запрос к NewsAPI и возвращает результаты.
func getNews(query string, pageSize, page int) (Results, error) {
	endpoint := fmt.Sprintf("https://newsapi.org/v2/everything?q=%s&pageSize=%d&page=%d&apiKey=%s&sortBy=publishedAt&language=en", url.QueryEscape(query), pageSize, page, *apiKey)
	log.Printf("Requesting URL: %s", endpoint) // Log the URL

	resp, err := http.Get(endpoint)
	if err != nil {
		log.Printf("HTTP Get error: %v", err) // Added logging
		return Results{}, fmt.Errorf("HTTP Get error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body) // Read the body for more info
		log.Printf("API status code error: %d, body: %s", resp.StatusCode, string(body))
		return Results{}, fmt.Errorf("API status code error: %d", resp.StatusCode) // More informative error
	}

	var results Results
	err = json.NewDecoder(resp.Body).Decode(&results)
	if err != nil {
		log.Printf("JSON decode error: %v", err) // Added logging
		return Results{}, fmt.Errorf("JSON decode error: %w", err)
	}

	return results, nil
}

func main() {
	// Load .env file (if it exists)
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file") // Non-fatal: allows apiKey to be passed via command line
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}

	apiKey = flag.String("apikey", os.Getenv("APIKEY"), "Newsapi.org access key")
	flag.Parse()

	if *apiKey == "" {
		log.Fatal("apiKey must be set") // Fatal: if no apiKey is provided
	}

	log.Printf("Using API key: %s (last 4 digits)", apiKeyHash(*apiKey)) // Добавил вывод для API key

	mux := http.NewServeMux()

	fs := http.FileServer(http.Dir("assets"))
	mux.Handle("/assets/", http.StripPrefix("/assets/", fs))

	mux.HandleFunc("/search", searchHandler)
	mux.HandleFunc("/", indexHandler)

	log.Printf("Server listening on port %s", port)
	err = http.ListenAndServe(":"+port, mux)
	if err != nil {
		log.Fatal("ListenAndServe error: ", err)
	}
}

func apiKeyHash(key string) string {
	if len(key) <= 4 {
		return "****" // If the key is too short, just return asterisks
	}
	return "********" + key[len(key)-4:] // Show only the last 4 characters
}
