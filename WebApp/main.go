package main

import (
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/GoIncremental/negroni-sessions/cookiestore"
	"github.com/goincremental/negroni-sessions"
	gmux "github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
	"github.com/urfave/negroni"
	"github.com/yosssi/ace"
	"golang.org/x/crypto/bcrypt"
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

type User struct {
	Username string `db:"username"`
	Secret   []byte `db:"secret"`
	Books    string `db:"books"`
}

type Page struct {
	Books []Book
	User  string
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

type UpdateBook struct {
	Book   Book
	Update bool
}

type LoginPage struct {
	Error string
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
	dbMap.AddTableWithName(User{}, "users").SetKeys(false, "username")
	err = dbMap.CreateTablesIfNotExists()
	if err != nil {
		return err
	}

	return nil
}

func getUserBookMap(books string) (mapBooks map[int64]bool) {
	strBooks := strings.Split(books, ",")
	mapBooks = make(map[int64]bool)
	for _, book := range strBooks {
		pk, _ := strconv.ParseInt(book, 10, 64)
		mapBooks[pk] = true
	}
	return
}

func getUserBooksFromMap(mapBooks map[int64]bool) (books string) {
	books = ""
	for pk := range mapBooks {
		books = books + fmt.Sprint(pk) + ","
	}
	return
}

func getStringFromSession(r *http.Request, key string) (value string) {
	val := sessions.GetSession(r).Get(key)
	if val != nil {
		value = val.(string)
	}
	return
}

func destroySession(r *http.Request) {
	sessions.GetSession(r).Set("User", nil)
	return
}

func verifyUser(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	if r.URL.Path == "/login" {
		log.Println("verifyUser :: path = /login")
		next(w, r)
		return
	}

	username := getStringFromSession(r, "User")
	log.Println("verifyUser :: username = ", username)
	user, _ := dbMap.Get(User{}, username)
	if user != nil {
		log.Println("verifyUser :: user found in session")
		next(w, r)
		return
	}
	log.Println("verifyUser :: user not found in session, redirecting to /login")
	http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
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
			User:  getStringFromSession(r, "User"),
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

	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		login := r.FormValue("login")
		signup := r.FormValue("signup")
		log.Println("/login login = ", login, " signup = ", signup)

		lp := LoginPage{""}

		username := r.FormValue("username")
		password := []byte(r.FormValue("password"))

		log.Println("/login username = ", username)

		if signup == "signup" {
			secret, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
			if err != nil {
				log.Println("/login error while encrypting password, error  ", err.Error())
				lp.Error = "Error while encrypting the password"
			} else {
				user := User{
					Username: username,
					Secret:   secret,
					Books:    "",
				}
				err = dbMap.Insert(&user)
				if err != nil {
					log.Println("/login Error while inserting user to database,  error = ", err.Error())
					lp.Error = "Error while adding user to database"
				} else {
					log.Println("/login New user created, username = ", username)
					sessions.GetSession(r).Set("User", user.Username)
					http.Redirect(w, r, "/", http.StatusFound)
					return
				}
			}

		} else if login == "login" {
			userInterface, err := dbMap.Get(User{}, username)
			if err != nil || userInterface == nil {
				log.Println("/login Error while retriving user info from database")
				lp.Error = "Error while retriving user info from database"
			} else {
				user := userInterface.(*User)
				err = bcrypt.CompareHashAndPassword(user.Secret, password)
				if err != nil {
					log.Println("/login Error while matching password, error = ", err.Error())
					lp.Error = "Password match error , error = " + err.Error()
				} else {
					log.Println("/login user found, username = ", username)
					sessions.GetSession(r).Set("User", user.Username)
					http.Redirect(w, r, "/", http.StatusFound)
					return
				}
			}
		} else {
		}

		template, err := ace.Load("templates/login", "", nil)
		if err != nil {
			log.Println("/login error while loading the template, error = ", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		err = template.Execute(w, lp)
		if err != nil {
			log.Println("/login error while executing the template, error = ", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

	}).Methods("GET", "POST")

	mux.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		destroySession(r)
		log.Println("/logout destroyed session, redirecting to /login")
		http.Redirect(w, r, "/login", http.StatusFound)
	}).Methods("POST")

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
		var userBook Book
		var user *User
		var ub UpdateBook
		ub.Update = false

		username := getStringFromSession(r, "User")

		err := dbMap.SelectOne(&userBook, "select * from books where id = ?", qs)
		if err != nil && err != sql.ErrNoRows {
			log.Println("/books/add id = ", qs, " error while retrieving book from database, error = ", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err == sql.ErrNoRows {
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
			err = dbMap.SelectOne(&userBook, "select * from books where id = ?", qs)
			if err != nil {
				log.Println("/books/add id = ", qs, " error while retrieving books from database, error = ", err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		userInterface, err := dbMap.Get(User{}, username)
		if err != nil {
			log.Println("/books/add id = ", qs, " error while retrieving user from database, error = ", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if userInterface == nil {
			log.Println("/books/add id = ", qs, " error while retrieving user from database, userInterface is nil")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		user = userInterface.(*User)
		_, present := getUserBookMap(user.Books)[userBook.PK]
		if !present {
			user.Books = user.Books + fmt.Sprint(userBook.PK) + ","
			_, err = dbMap.Update(&user)
			if err != nil {
				log.Println("/books/add id = ", qs, " error while updating user table, error = ", err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			ub.Book = userBook
			ub.Update = true
		}

		encoder := json.NewEncoder(w)
		err = encoder.Encode(ub)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}).Methods("POST", "PUT")

	mux.HandleFunc("/books/{pk}", func(w http.ResponseWriter, r *http.Request) {
		pk := gmux.Vars(r)["pk"]
		username := getStringFromSession(r, "User")
		log.Println("/books/delete => pk = ", pk)

		pkInt64, err := strconv.ParseInt(pk, 10, 64)
		if err != nil {
			log.Println("/books/delete pk = ", pk, " Error while parsing pk, error = ", err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		userInterface, err := dbMap.Get(User{}, username)
		if err != nil {
			log.Println("/books/delete pk = ", pk, " Error while retriving user info, error = ", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if userInterface == nil {
			log.Println("/books/delete pk = ", pk, " Error while retriving user info, error = ", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		user := userInterface.(*User)
		userBookMap := getUserBookMap(user.Books)
		_, found := userBookMap[pkInt64]
		if found {
			delete(userBookMap, pkInt64)
			user.Books = getUserBooksFromMap(userBookMap)
			_, err = dbMap.Update(&user)
			if err != nil {
				log.Println("/books/delete pk = ", pk, " Error while updating user info, error = ", err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}).Methods("DELETE")

	n := negroni.Classic()
	n.Use(sessions.Sessions("go-web-development", cookiestore.New([]byte("my-secret-123"))))
	n.Use(negroni.HandlerFunc(verifyDBConnection))
	n.Use(negroni.HandlerFunc(verifyUser))
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
