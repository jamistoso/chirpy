package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jamistoso/chirpy/internal/database"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries *database.Queries
	platform string
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

	if cfg.platform != "dev" {
		respondWithError(rWriter, 403, "reset attempted on non-dev platform")
		return
	}
	cfg.dbQueries.Reset(rq.Context())
	fmt.Println("Reset")
	cfg.fileserverHits.Store(0)
	respondWithJSON(rWriter, 200, "File server hits reset to 0")
}

func (cfg *apiConfig) usersHandler(rWriter http.ResponseWriter, rq *http.Request) {
	type parameters struct {
		Email string `json:"email"`
	}

	decoder := json.NewDecoder(rq.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(rWriter, 500, "Error decoding parameters")
		return
	}

	dbUser, err := cfg.dbQueries.CreateUser(rq.Context(), params.Email,)
	if err != nil {
		respondWithError(rWriter, 500, "Error creating user")
		return
	}

	type returnVals struct {
		Id 			uuid.UUID	`json:"id"`
		Created_at 	time.Time 	`json:"created_at"`
		Updated_at 	time.Time 	`json:"updated_at"`
		Email		string		`json:"email"`
	}

	respBody := returnVals{
		Id:			dbUser.ID,
		Created_at: dbUser.CreatedAt,
		Updated_at: dbUser.UpdatedAt,
		Email:		dbUser.Email,
	}
	
	respondWithJSON(rWriter, 201, respBody)
}

func (cfg *apiConfig) chirpsHandler(rWriter http.ResponseWriter, rq *http.Request) {
	type parameters struct {
		Body 	string `json:"body"`
		User_id uuid.UUID `json:"user_id"`
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

	chirp, err := cfg.dbQueries.CreateChirp(rq.Context(), database.CreateChirpParams{
		Body: params.Body,
		UserID: uuid.NullUUID{
			UUID: params.User_id,
			Valid: true,
		},
	})

	if err != nil {
		respondWithError(rWriter, 500, "Error creating chirp")
		return
	}

	type returnVals struct {
		ID 			uuid.UUID	`json:"id"`
		Created_at 	time.Time 	`json:"created_at"`
		Updated_at	time.Time	`json:"updated_at"`
		Body		string		`json:"body"`
		User_id		uuid.UUID	`json:"user_id"`
	}

	respBody := returnVals{
		ID: 		chirp.ID,
		Created_at: chirp.CreatedAt,
		Updated_at: chirp.UpdatedAt,
		Body: 		chirp.Body,
		User_id: 	chirp.UserID.UUID,
	}
	
	respondWithJSON(rWriter, 201, respBody)
}

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Println(err)
		return
	}
	serveMux := http.NewServeMux()

	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	
	if err != nil {
		fmt.Println(err)
		return
	}

	dbQueries := database.New(db)
	platform := os.Getenv("PLATFORM")

	apiCfg := &apiConfig{
		fileserverHits:	atomic.Int32{},
		dbQueries: 		dbQueries,
		platform:		platform,	
	}
	serveHandler := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	serveMux.Handle("/app/", apiCfg.middlewareMetricsInc(serveHandler))

	healthHandler := func(rWriter http.ResponseWriter, rq *http.Request) {
		rWriter.Header().Add("Content-Type", "text/plain; charset=utf-8" )
		rWriter.WriteHeader(200)
		rWriter.Write([]byte("OK"))
	}
	serveMux.HandleFunc("GET /api/healthz", healthHandler)

	serveMux.HandleFunc("POST /api/chirps", apiCfg.chirpsHandler)

	serveMux.HandleFunc("POST /api/users", apiCfg.usersHandler)

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