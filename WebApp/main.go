package main

import (
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"text/template"

	_ "github.com/mattn/go-sqlite3"
	"github.com/urfave/negroni"
)

var port = ":8080"

type Page struct {
	Name     string
	DBStatus bool
}

type SearchResult struct {
	Title  string `xml:"title,attr"`
	Author string `xml:"author,attr"`
	Year   string `xml:"hyr,attr"`
	ID     string `xml:"owi,attr"`
}

type ClassifySearchResponse struct {
	Results []SearchResult `xml:"works>work"`
}

type ClassifyBookResponse struct {
	BookData struct {
		Title  string `xml:"title,attr"`
		Author string `xml:"author,attr"`
		ID     string `xml:"owi,attr"`
	} `xml:"work"`
	Classification struct {
		MostPopular string `xml:"sfa,attr"`
	} `xml:"recommandations>ddc>mostPopular"`
}

var db *sql.DB

func verifyDBConnection(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	err := db.Ping()
	if err != nil {
		log.Println("verifyDBConnection :: DB not connected")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		next(w, r)
	}

}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Llongfile)

	templates := template.Must(template.ParseFiles("templates/index.html"))

	db, _ = sql.Open("sqlite3", "dev.db")

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		err := templates.ExecuteTemplate(w, "index.html", nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		qs := r.FormValue("queryString")
		log.Println("/search => qs = ", qs)
		results, err := search(qs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		encoder := json.NewEncoder(w)
		err = encoder.Encode(results)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	mux.HandleFunc("/books/add", func(w http.ResponseWriter, r *http.Request) {
		qs := r.FormValue("id")
		log.Println("/book/add => qs = ", qs)

		book, err := find(qs)
		if err != nil {
			log.Println("/books/add qs = ", qs, " error while finding ", " error = ", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if book.BookData.Title == "" {
			log.Println("/books/add qs = ", qs, " This book is not popular")
			http.Error(w, "This book is not popular", http.StatusNoContent)
			return
		}

		_, err = db.Exec("insert into books (pk, title, author, id, classification) values (?, ?, ?, ?, ?)",
			nil, book.BookData.Title, book.BookData.Author, book.BookData.ID, book.Classification.MostPopular)
		if err != nil {
			log.Println("/books/add qs = ", qs, " error while inserting into DB error = ", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Println("/books/add qs = ", qs, " successfully inserted into db")

		encoder := json.NewEncoder(w)
		err = encoder.Encode(book)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	n := negroni.Classic()
	n.Use(negroni.HandlerFunc(verifyDBConnection))
	n.UseHandler(mux)
	n.Run(port)
}

func search(query string) (results []SearchResult, err error) {
	var searchURL = "http://classify.oclc.org/classify2/Classify?&summary=true&title="
	var body []byte
	var csr ClassifySearchResponse

	searchURL = searchURL + url.QueryEscape(query)

	log.Println("func search ::url = ", searchURL)

	body, err = classifyAPI(searchURL)
	if err != nil {
		log.Println("func search :: err while requesting ", "url = ", searchURL, " error = ", err.Error())
		return
	}

	err = xml.Unmarshal(body, &csr)
	if err != nil {
		log.Println("func search :: err while Unmarshalling ", "url = ", searchURL, " error = ", err.Error())
		return
	}
	results = csr.Results

	return
}

func find(id string) (cbr ClassifyBookResponse, err error) {
	var searchURL = "http://classify.oclc.org/classify2/Classify?&summary=true&owi="
	var body []byte

	searchURL = searchURL + url.QueryEscape(id)

	log.Println("func find ::url = ", searchURL)

	body, err = classifyAPI(searchURL)
	if err != nil {
		log.Println("func find :: err while requesting ", "url = ", searchURL, " error = ", err.Error())
		return
	}

	//log.Println("func find ::url = ", searchURL, " obtained body body = ", string(body))

	err = xml.Unmarshal(body, &cbr)
	if err != nil {
		log.Println("func find :: err while Unmarshalling ", "url = ", searchURL, " error = ", err.Error())
		return
	}

	log.Println("func find ::url = ", searchURL, " successfully unmarshalled cbr = ", cbr)

	return
}

func classifyAPI(url string) (body []byte, err error) {
	var resp *http.Response

	resp, err = http.Get(url)
	if err != nil {
		log.Println("func classifyAPI :: err while requesting ", "url = ", url, " error = ", err.Error())
		return
	}
	defer resp.Body.Close()

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("func classifyAPI :: err while parsing the body ", "url = ", url, " error = ", err.Error())
		return
	}

	return
}
