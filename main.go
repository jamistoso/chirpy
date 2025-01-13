package main

import (
	"fmt"
	"log"
	"strings"
	"encoding/json"
	"sync/atomic"
	"net/http"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) metricsHandler(rWriter http.ResponseWriter, rq *http.Request) {
	rWriter.Header().Add("Content-Type", "text/html" )
	rWriter.Write([]byte(fmt.Sprintf(`<html>
  	<body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p></body>
	</html>`, cfg.fileserverHits.Load())))
}

func (cfg *apiConfig) resetHandler(rWriter http.ResponseWriter, rq *http.Request) {
	fmt.Println("Reset")
	cfg.fileserverHits.Store(0)
	rWriter.Write([]byte("File server hits reset to 0"))
}

func main() {
	serveMux := http.NewServeMux()

	apiCfg := &apiConfig{
		fileserverHits:	atomic.Int32{},
	}
	serveHandler := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	serveMux.Handle("/app/", apiCfg.middlewareMetricsInc(serveHandler))

	healthHandler := func(rWriter http.ResponseWriter, rq *http.Request) {
		rWriter.Header().Add("Content-Type", "text/plain; charset=utf-8" )
		rWriter.WriteHeader(200)
		rWriter.Write([]byte("OK"))
	}
	serveMux.HandleFunc("GET /api/healthz", healthHandler)

	validateHandler := func(rWriter http.ResponseWriter, rq *http.Request) {
		type parameters struct {
			Body string `json:"body"`
		}
	
		decoder := json.NewDecoder(rq.Body)
		params := parameters{}
		err := decoder.Decode(&params)
		if err != nil {
			respondWithError(rWriter, 500, "Error decoding parameters")
			return
		}

		if len(params.Body) > 140 {
			respondWithError(rWriter, 400, "Chirp is too long")
			return
		}

		params.Body = profanityFilter(params.Body)

		type returnVals struct {
			Valid bool			`json:"valid"`
			Cleaned_body string `json:"cleaned_body"`
		}

		respBody := returnVals{
			Valid: true,
			Cleaned_body: params.Body,
		}
		
		respondWithJSON(rWriter, 200, respBody)
	}

	serveMux.HandleFunc("POST /api/validate_chirp", validateHandler)

	serveMux.HandleFunc("GET /admin/metrics", apiCfg.metricsHandler)
	
	serveMux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)

	server := &http.Server{
		Handler:	serveMux,
		Addr: 		":8080",	
	}
	server.ListenAndServe()

}

func respondWithError(rWriter http.ResponseWriter, code int, msg string) {
	rWriter.Header().Set("Content-Type", "application/json")
	rWriter.WriteHeader(code)
	rWriter.Write([]byte(msg))
}

func respondWithJSON(rWriter http.ResponseWriter, code int, payload interface{}) {
	dat, err := json.Marshal(payload)
	if err != nil {
			log.Printf("Error marshalling JSON: %s", err)
			rWriter.WriteHeader(500)
			return
	}
	rWriter.Header().Set("Content-Type", "application/json")
	rWriter.WriteHeader(code)
	rWriter.Write(dat)
}

func profanityFilter(msg string) string {
	filterMap := map[string]string{
		"kerfuffle": "", 
		"sharbert": "", 
		"fornax": ""}
	msgSplit := strings.Split(msg, " ")
	for i := 0; i < len(msgSplit); i++ {
		if _, ok := filterMap[strings.ToLower(msgSplit[i])]; ok {
			msgSplit[i] = "****"
		}
	}
	return strings.Join(msgSplit, " ")
}