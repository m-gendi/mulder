package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/gomodule/redigo/redis"
)

const (
	quotesKey = "quotes"
)

var (
	quotes = []interface{}{ // https://www.imdb.com/title/tt0106179/quotes
		"Je voudrais déjà être roi",
		"Libérée Delivrée je ne mentirais plus jamais",
		"Un jour mon prince viendras",
		"Hakuna Matata",
	}

	listenAddr          string
	redisAddr           string
	redisConnectTimeout time.Duration

	redisConn redis.Conn
)

func init() {
	rand.Seed(time.Now().UnixNano())

	flag.StringVar(&listenAddr, "listen-addr", ":8080", "host:port on which to listen")
	flag.StringVar(&redisAddr, "redis-addr", ":6379", "redis host:port to connect to")
	flag.DurationVar(&redisConnectTimeout, "redis-connect-timeout", 1*time.Minute, "timeout for connecting to redis")

	http.HandleFunc("/quote/random", randomQuoteHandler)
	http.HandleFunc("/healthz", healthzHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {})
}

func main() {
	log.Println("Mulder is waking up...")
	flag.Parse()

	if err := connectToRedis(); err != nil {
		log.Fatalf("Failed to connect to The (redis) X-Files at %s after timeout %s: %v", redisAddr, redisConnectTimeout, err)
	}
	defer redisConn.Close()

	if err := insertQuotesInRedis(); err != nil {
		log.Fatalf("Failed to insert files in The X-Files: %v", err)
	}

	log.Printf("Starting HTTP server on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		log.Fatalf("Failed to listen on %s: %v", listenAddr, err)
	}
}

func connectToRedis() (err error) {
	log.Printf("Connecting to The (redis) X-Files at %s...", redisAddr)

	redisConn, err = redis.Dial("tcp", redisAddr, redis.DialConnectTimeout(redisConnectTimeout))
	if err != nil {
		return err
	}

	infos, err := redis.String(redisConn.Do("INFO", "SERVER"))
	if err != nil {
		return err
	}

	log.Printf("Connected to The (redis) X-Files:\n%s", infos)
	return nil
}

func insertQuotesInRedis() error {
	log.Println("Checking The X-Files...")
	existingQuotes, err := redis.Int(redisConn.Do("LLEN", quotesKey))
	if err != nil {
		return err
	}

	if existingQuotes == len(quotes) {
		log.Printf("All The %d X-Files are already there!", existingQuotes)
		return nil
	}

	if existingQuotes > 0 {
		log.Printf("There is a mess in The X-Files, we don't have the right number of quotes - %d instead of %d. Let's clean everything first...", existingQuotes, len(quotes))
		if _, err = redis.Int(redisConn.Do("DEL", quotesKey)); err != nil {
			return err
		}
	}

	log.Printf("Inserting %d files in The X-Files...", len(quotes))
	args := append([]interface{}{}, quotesKey)
	args = append(args, quotes...)
	insertedQuotes, err := redis.Int(redisConn.Do("RPUSH", args...))
	if err != nil {
		return err
	}

	log.Printf("Inserted %d/%d files in The X-Files", insertedQuotes, len(quotes))
	return nil
}

func getRandomQuote() (string, error) {
	quotesCount, err := redis.Int(redisConn.Do("LLEN", quotesKey))
	if err != nil {
		return "", err
	}

	randomIndex := rand.Intn(quotesCount)
	return redis.String(redisConn.Do("LINDEX", quotesKey, randomIndex))
}

func randomQuoteHandler(w http.ResponseWriter, r *http.Request) {
	quote, err := getRandomQuote()
	if err != nil {
		log.Printf("Failed to retrieve an X-File: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Handled an X-File request, returned: '%s'", quote)

	w.Header().Set("Content-Type", "application/json")
	if err = json.NewEncoder(w).Encode(&response{Quote: quote}); err != nil {
		log.Printf("Failed to write HTTP response: %v", err)
	}
}

type response struct {
	Quote string `json:"quote"`
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	pong, err := redis.String(redisConn.Do("PING"))
	if err != nil {
		log.Printf("Healthz handler failing: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	io.WriteString(w, pong)
}
