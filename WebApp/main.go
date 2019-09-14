package main

import (
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"

	gmux "github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
	"github.com/urfave/negroni"
	"github.com/yosssi/ace"
	"gopkg.in/gorp.v1"
)

var port = ":8080"

type Book struct {
	PK             int64  `db:"pk"`
	Title          string `db:"title"`
	Author         string `db:"author"`
	Classification string `db:"classification"`
	ID             string `db:"id"`
}

type Page struct {
	Books []Book
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
var dbMap *gorp.DbMap

func initDB() (err error) {
	db, err = sql.Open("sqlite3", "dev.db")
	if err != nil {
		return err
	}

	dbMap = &gorp.DbMap{
		Db:      db,
		Dialect: gorp.SqliteDialect{},
	}

	dbMap.AddTableWithName(Book{}, "books").SetKeys(true, "pk")
	err = dbMap.CreateTablesIfNotExists()
	if err != nil {
		return err
	}
	return nil
}

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

	template, err := ace.Load("templates/index", "", nil)
	if err != nil {
		log.Println("func main :: error while loading template error = ", err.Error())
		return
	}

	err = initDB()
	if err != nil {
		log.Println("func main :: error from initDB() error = ", err.Error())
		return
	}

	//mux := http.NewServeMux()
	mux := gmux.NewRouter()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := Page{
			Books: []Book{},
		}

		_, err := dbMap.Select(&p.Books, "select * from books")
		if err != nil {
			log.Println("func main :: error while fetching books from database")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		err = template.Execute(w, p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}).Methods("GET")

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
	}).Methods("POST")

	mux.HandleFunc("/books/{id}", func(w http.ResponseWriter, r *http.Request) {
		qs := gmux.Vars(r)["id"]
		log.Println("/books/add => qs = ", qs)

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

		b := Book{
			PK:             -1,
			Title:          book.BookData.Title,
			Author:         book.BookData.Author,
			Classification: book.Classification.MostPopular,
			ID:             book.BookData.ID,
		}

		err = dbMap.Insert(&b)
		if err != nil {
			log.Println("/books/add qs = ", qs, " error while inserting into DB error = ", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		encoder := json.NewEncoder(w)
		err = encoder.Encode(b)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}).Methods("POST", "PUT")

	mux.HandleFunc("/books/{pk}", func(w http.ResponseWriter, r *http.Request) {
		pk := gmux.Vars(r)["pk"]
		log.Println("/books/delete => pk = ", pk)

		pkInt64, err := strconv.ParseInt(pk, 10, 64)
		if err != nil {
			log.Println("/books/delete pk = ", pk, " Error while parsing pk, error = ", err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		_, err = dbMap.Delete(&Book{pkInt64, "", "", "", ""})
		if err != nil {
			log.Println("/boks/delete pk = ", pk, " Error while deleting the book, error = ", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}).Methods("DELETE")

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
