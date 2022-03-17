package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

var (
	ActiveRouter  *mux.Router
	backend       string
	latestRequest int
)

type Message struct {
	id            string
	username      string
	insertionDate int64
	tweet         string
	flagged       bool
}

func userExists(username string) bool {
	response := requestToBackend("/user/get-user/"+username, "GET", nil)
	defer response.Body.Close()
	status := response.StatusCode
	if status == http.StatusOK {
		return true
	} else {
		return false
	}
}

func filterMessages(messages []Message, n int) []byte {

	filteredMessages := []map[string]string{}
	for _, msg := range messages[:n] {
		filteredMessages = append(filteredMessages, map[string]string{"content": msg.tweet, "pub_date": fmt.Sprint(msg.insertionDate), "username": msg.username})
	}
	res, err := json.Marshal(filteredMessages)
	if err != nil {
		log.Println(err)
	}
	return res
}

func requestToBackend(endpoint, method string, data []byte) *http.Response {
	client := http.Client{Timeout: time.Second * 50}

	req, err := http.NewRequest(method, backend+endpoint, bytes.NewBuffer(data))
	req.Header.Set("content-type", "application/JSON")
	if err != nil {
		log.Println(err)
	}

	response, err := client.Do(req)
	if err != nil || response == nil {
		log.Println(err)
	}
	//log.Println(endpoint, method, response.Status)
	return response
}

func checkAuthKey(r *http.Request) []byte {
	authKey := r.Header.Get("Authorization")

	if authKey != "Basic c2ltdWxhdG9yOnN1cGVyX3NhZmUh" {
		err := "You are not authorized to use this resource!"
		res, _ := json.Marshal(map[string]interface{}{"status": 403, "error_msg": err})
		return res
	}
	return nil
}

func updateLatest(vals url.Values) {
	temp, err := strconv.Atoi(vals.Get("latest"))
	if err != nil {
		latestRequest = -1
	} else {
		latestRequest = temp
	}
}

func main() {
	ActiveRouter = mux.NewRouter()

	gob.Register(map[string]string{})
	// prepare commandline flags
	var addressAndPort string
	var listenPort string
	flag.StringVar(&addressAndPort, "backend", "localhost:8080", "the address of the backend api")
	flag.StringVar(&listenPort, "port", "9000", "the address to serve the api on")
	flag.Parse()
	backend = fmt.Sprintf("http://%s/devops", addressAndPort)

	log.Println(backend)

	// prepare routes
	ActiveRouter.HandleFunc("/latest", latest).Methods("GET").Name("latest")
	ActiveRouter.HandleFunc("/register", register).Methods("POST").Name("register")
	ActiveRouter.HandleFunc("/msgs", msgs).Methods("GET").Name("msgs")
	ActiveRouter.HandleFunc("/msgs/{username}", msgsUser).Methods("GET", "POST").Name("msgsUser")
	ActiveRouter.HandleFunc("/fllws/{username}", fllws).Methods("GET", "POST").Name("fllws")

	log.Println("Listening on port " + listenPort)
	log.Fatal(http.ListenAndServe(":"+listenPort, ActiveRouter))

}

func latest(w http.ResponseWriter, r *http.Request) {
	//log.Println("latest enter")
	res, err := json.Marshal(map[string]int{"latest": latestRequest})
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(res)
}

func register(w http.ResponseWriter, r *http.Request) {
	//log.Println("register enter")
	vals := r.URL.Query()
	updateLatest(vals)
	errorMsg := ""
	errorStatus := 400
	if r.Method == "POST" {
		r.ParseForm()
		//log.Println(r.Form)
		var data map[string]string

		json.NewDecoder(r.Body).Decode(&data)
		if data["username"] == "" {
			errorMsg = "You have to enter a username"
		} else if data["email"] == "" || !strings.Contains(data["email"], "@") {
			errorMsg = "You have to enter a valid email address"
		} else if data["pwd"] == "" {
			errorMsg = "You have to enter a password"
		} else if userExists(data["username"]) {
			errorMsg = "The username is already taken"
			errorStatus = 409
		} else {
			/*
				hashed, err := bcrypt.GenerateFromPassword([]byte(data["pwd"]), bcrypt.DefaultCost)
				if err != nil {
					log.Println(err)
				}
			*/
			requestBody, err := json.Marshal(map[string]interface{}{"username": data["username"], "email": data["email"], "password": data["pwd"], "isAdmin": false}) //Figure out hash algorithm
			if err != nil {
				log.Println(err)
			}

			response := requestToBackend("/user/register", "POST", requestBody)
			defer response.Body.Close()
			//log.Println(response.Status)
			status := response.StatusCode
			if status == 400 {
				errorMsg = "User could not be created"
			} else if status == 500 {
				errorMsg = "Internal Server Error"
				errorStatus = 500
			}
		}
	}
	//log.Println("Error:", err)

	if errorMsg != "" {
		log.Print(errorMsg)
		res, _ := json.Marshal(map[string]interface{}{"status": errorStatus, "error_msg": errorMsg})
		w.Write(res)
	} else {
		w.WriteHeader(http.StatusNoContent)
		w.Write(nil)
	}
}

func msgs(w http.ResponseWriter, r *http.Request) {
	//log.Println("msgs enter")
	vals := r.URL.Query()
	updateLatest(vals)

	authorized := checkAuthKey(r)
	if authorized != nil {
		w.Write(authorized)
		return
	}

	n := 100
	temp := vals.Get("no")
	tmp, err := strconv.Atoi(temp)
	if err == nil && tmp > 0 {
		n = tmp
	}

	if r.Method == "GET" {

		var data []Message
		response := requestToBackend(fmt.Sprintf("/tweet/get-all-tweets/%v/0", n), "GET", nil)
		defer response.Body.Close()
		json.NewDecoder(response.Body).Decode(&data)
		status := response.StatusCode

		errorMsg := ""
		if status == 500 {
			errorMsg = "Internal Server Error"
		} else if status != http.StatusOK {
			errorMsg = "Could not fetch tweets"
		}

		if errorMsg != "" {
			log.Print(errorMsg)
			res, _ := json.Marshal(map[string]interface{}{"status": status, "error_msg": errorMsg})
			w.Write(res)
		} else {
			filtered := filterMessages(data, n)
			w.Write(filtered)
			return
		}
	}
}

func msgsUser(w http.ResponseWriter, r *http.Request) {
	//log.Println("msgsUser enter")
	vals := r.URL.Query()
	updateLatest(vals)
	username := mux.Vars(r)["username"]

	authorized := checkAuthKey(r)
	if authorized != nil {
		w.Write(authorized)
		return
	}

	n := 100
	temp := vals.Get("no")
	tmp, err := strconv.Atoi(temp)
	if err == nil && tmp > 0 {
		n = tmp
	}

	if r.Method == "GET" {
		if !userExists(username) {
			http.Error(w, "", 404)
			return
		}

		var data []Message
		response := requestToBackend("/tweet/get-user-tweets/"+username, "GET", nil)
		defer response.Body.Close()
		json.NewDecoder(response.Body).Decode(&data)
		status := response.StatusCode
		errorMsg := ""
		if status == 500 {
			errorMsg = "Internal Server Error"
		} else if status != http.StatusOK {
			errorMsg = "Could not fetch tweets"
		}

		if errorMsg != "" {
			log.Print(errorMsg)
			res, _ := json.Marshal(map[string]interface{}{"status": status, "error_msg": errorMsg})
			w.Write(res)
		} else {
			filtered := filterMessages(data, n)
			w.Write(filtered)
		}
		return
	}

	if r.Method == "POST" {

		data := struct {
			Username string
			Content  string
		}{}

		json.NewDecoder(r.Body).Decode(&data)

		newTweet := map[string]interface{}{"username": username, "tweet": data.Content, "insertionDate": time.Now().UTC()}
		res, _ := json.Marshal(newTweet)

		response := requestToBackend("/tweet/add-tweet", "POST", res)
		defer response.Body.Close()
		status := response.StatusCode
		errorMsg := ""
		if status == 500 {
			errorMsg = "Internal Server Error"
		} else if status != http.StatusOK {
			errorMsg = "Could not create tweet"
		}

		if errorMsg != "" {
			log.Print(errorMsg)
			res, _ := json.Marshal(map[string]interface{}{"status": status, "error_msg": errorMsg})
			w.Write(res)
		} else {
			w.WriteHeader(http.StatusNoContent)
			w.Write(nil)
		}
		return
	}
}

func fllws(w http.ResponseWriter, r *http.Request) {
	//log.Println("fllwsUser enter")
	vals := r.URL.Query()
	updateLatest(vals)
	username := mux.Vars(r)["username"]

	authorized := checkAuthKey(r)
	if authorized != nil {
		w.Write(authorized)
		return
	}

	if !userExists(username) {
		http.Error(w, "", 404)
		return
	}

	n := 100
	temp := vals.Get("no")
	tmp, err := strconv.Atoi(temp)
	if err == nil && tmp > 0 {
		n = tmp
	}

	var data map[string]string

	json.NewDecoder(r.Body).Decode(&data)

	if r.Method == "POST" && data["follow"] != "" {
		targetUsername := data["follow"]
		if !userExists(targetUsername) {
			http.Error(w, "", 404)
			return
		}

		data := map[string]string{"currentUsername": username, "targetUsername": targetUsername}
		res, _ := json.Marshal(data)
		response := requestToBackend("/user/follow", "POST", res)
		defer response.Body.Close()
		status := response.StatusCode
		errorMsg := ""
		if status == 500 {
			errorMsg = "Internal Server Error"
		} else if status != http.StatusOK {
			errorMsg = "Could not follow"
		}

		if errorMsg != "" {
			log.Print(errorMsg)
			res, _ := json.Marshal(map[string]interface{}{"status": status, "error_msg": errorMsg})
			w.Write(res)
		} else {

			w.WriteHeader(204)
			w.Write(nil)
		}
		return
	} else if r.Method == "POST" && data["unfollow"] != "" {
		targetUsername := data["unfollow"]
		if !userExists(targetUsername) {
			http.Error(w, "", 404)
			return
		}

		data := map[string]string{"currentUsername": username, "targetUsername": targetUsername}
		res, _ := json.Marshal(data)
		response := requestToBackend("/user/unfollow", "POST", res)
		defer response.Body.Close()
		status := response.StatusCode
		errorMsg := ""
		if status == 500 {
			errorMsg = "Internal Server Error"
		} else if status != http.StatusOK {
			errorMsg = "Could not unfollow"
		}

		if errorMsg != "" {
			log.Print(errorMsg)
			res, _ := json.Marshal(map[string]interface{}{"status": status, "error_msg": errorMsg})
			w.Write(res)
		} else {

			w.WriteHeader(204)
			w.Write(nil)
		}
		return

	} else if r.Method == "GET" {

		data := map[string]string{"currentUsername": username}
		res, _ := json.Marshal(data)

		partialUser := map[string][]string{"following": nil}
		response := requestToBackend("/user/get-user/"+username, "GET", res)
		defer response.Body.Close()
		json.NewDecoder(response.Body).Decode(&partialUser)
		status := response.StatusCode
		errorMsg := ""
		if status == 500 {
			errorMsg = "Internal Server Error"
		} else if status == 404 {
			errorMsg = "Username not found"
		} else if status != http.StatusOK {
			errorMsg = "Could not get follows"
		}

		if errorMsg != "" {
			log.Print(errorMsg)
			res, _ := json.Marshal(map[string]interface{}{"status": status, "error_msg": errorMsg})
			w.Write(res)
			return
		}

		//log.Println("partialUser", partialUser)
		following := partialUser["following"]
		m := len(following)
		if n > m {
			n = m
		}

		res, err := json.Marshal(map[string][]string{"follows": following[:n]})
		if err != nil {
			log.Println(err)
		}
		w.Write(res)
	}
}
