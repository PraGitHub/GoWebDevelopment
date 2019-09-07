package main

import (
	"encoding/json"
	"log"
	"net/http"
	"text/template"

	_ "github.com/mattn/go-sqlite3"
)

var port = ":8080"

type Page struct {
	Name     string
	DBStatus bool
}

type SearchResult struct {
	Title  string
	Author string
	Year   string
	ID     string
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Llongfile)

	templates := template.Must(template.ParseFiles("templates/index.html"))

	//db, _ := sql.Open("sqlite3", "dev.db")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		err := templates.ExecuteTemplate(w, "index.html", nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		results := []SearchResult{
			SearchResult{"Nagarahvu", "T R Subbarao", "1972", "51JeF5abJuXT1zoNjlJe"},
			SearchResult{"And then ther were none", "Agatha Christie", "1939", "eeSqQGbnJxMgQSk9oCDL"},
			SearchResult{"IT", "Stephen King", "1986", "ZOlu3Nvwxz6vlTjDwLtE"},
			SearchResult{"Eradu Kanasu", "Vani", "1974", "9YVvLgV8dN60DMg0HTGn"},
		}
		encoder := json.NewEncoder(w)
		err := encoder.Encode(results)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	log.Println(http.ListenAndServe(port, nil))
}
