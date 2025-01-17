package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jamistoso/chirpy/internal/auth"
	"github.com/jamistoso/chirpy/internal/database"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits 	atomic.Int32
	dbQueries 		*database.Queries
	platform 		string
	jwtSecret 		string
	polkaKey		string
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
		Password 	string `json:"password"`
		Email 		string `json:"email"`
	}

	decoder := json.NewDecoder(rq.Body)
	rqParams := parameters{}
	err := decoder.Decode(&rqParams)
	if err != nil {
		respondWithError(rWriter, 500, "Error decoding parameters")
		return
	}

	hashPass, err := auth.HashPassword(rqParams.Password)
	if err != nil {
		respondWithError(rWriter, 500, "Error hashing password")
	}

	userParams := database.CreateUserParams{
		Email:			rqParams.Email,
		HashedPassword: hashPass,
	}

	dbUser, err := cfg.dbQueries.CreateUser(rq.Context(), userParams)
	if err != nil {
		respondWithError(rWriter, 500, "Error creating user")
		return
	}

	type returnVals struct {
		Id 					uuid.UUID	`json:"id"`
		Created_at 			time.Time 	`json:"created_at"`
		Updated_at 			time.Time 	`json:"updated_at"`
		Email				string		`json:"email"`
		Is_chirpy_red		bool		`json:"is_chirpy_red"`
	}

	respBody := returnVals{
		Id:					dbUser.ID,
		Created_at: 		dbUser.CreatedAt,
		Updated_at: 		dbUser.UpdatedAt,
		Email:				dbUser.Email,
		Is_chirpy_red:		dbUser.IsChirpyRed.Bool,
	}
	
	respondWithJSON(rWriter, 201, respBody)
}

func (cfg *apiConfig) loginHandler(rWriter http.ResponseWriter, rq *http.Request) {
	type parameters struct {
		Password 			string 	`json:"password"`
		Email 				string 	`json:"email"`
	}

	decoder := json.NewDecoder(rq.Body)
	rqParams := parameters{}
	err := decoder.Decode(&rqParams)
	if err != nil {
		respondWithError(rWriter, 500, "error decoding parameters")
		return
	}

	dbUser, err := cfg.dbQueries.GetUserFromEmail(rq.Context(), rqParams.Email)
	if err != nil {
		respondWithError(rWriter, 500, "error creating user")
		return
	}

	err = auth.CheckPasswordHash(rqParams.Password, dbUser.HashedPassword)
	if err != nil {
		respondWithError(rWriter, 401, "incorrect email or password")
		return
	}

	jwtToken, err := auth.MakeJWT(dbUser.ID, cfg.jwtSecret, time.Duration(1) * time.Hour)
	if err != nil {
		respondWithError(rWriter, 500, "jwt token creation failed")
		return
	}

	refreshToken, err := auth.MakeRefreshToken()
	if err != nil {
		respondWithError(rWriter, 500, "refresh token creation failed")
		return
	}

	refreshExpirationTime := time.Now().Add(time.Duration(60) * time.Hour * 24)

	params := database.CreateRefreshTokenParams{
		Token:	refreshToken,
		UserID:	uuid.NullUUID{
			UUID:	dbUser.ID,
			Valid:	true,
		},
		ExpiresAt: refreshExpirationTime,
	}

	_, err = cfg.dbQueries.CreateRefreshToken(rq.Context(), params)
	if err != nil {
		respondWithError(rWriter, 500, err.Error())
		return
	}

	type returnVals struct {
		Id 					uuid.UUID	`json:"id"`
		Created_at 			time.Time 	`json:"created_at"`
		Updated_at 			time.Time 	`json:"updated_at"`
		Email				string		`json:"email"`
		Token				string		`json:"token"`
		Refresh_token		string		`json:"refresh_token"`
		Is_chirpy_red		bool		`json:"is_chirpy_red"`
	}

	respBody := returnVals{
		Id:					dbUser.ID,
		Created_at: 		dbUser.CreatedAt,
		Updated_at: 		dbUser.UpdatedAt,
		Email:				dbUser.Email,
		Token:				jwtToken,
		Refresh_token:		refreshToken,
		Is_chirpy_red:		dbUser.IsChirpyRed.Bool,
	}
	
	respondWithJSON(rWriter, 200, respBody)
}

func (cfg *apiConfig) refreshHandler(rWriter http.ResponseWriter, rq *http.Request) {
	refreshToken, err := auth.GetBearerToken(rq.Header)
	if err != nil {
		respondWithError(rWriter, 401, err.Error())
	}

	dbToken, err := cfg.dbQueries.GetUserFromRefreshToken(rq.Context(), refreshToken)
	if err != nil {
		respondWithError(rWriter, 401, err.Error())
	}
	if dbToken.RevokedAt.Valid {
		respondWithError(rWriter, 401, "invalid refresh token")
	}

	jwtToken, err := auth.MakeJWT(dbToken.UserID.UUID, cfg.jwtSecret, time.Duration(1) * time.Hour)
	if err != nil {
		respondWithError(rWriter, 500, "jwt token creation failed")
		return
	}

	type returnVals struct {
		Token	string	`json:"token"`
	}

	respBody := returnVals{
		Token:	jwtToken,
	}

	respondWithJSON(rWriter, 200, respBody)
}

func (cfg *apiConfig) revokeHandler(rWriter http.ResponseWriter, rq *http.Request) {
	refreshToken, err := auth.GetBearerToken(rq.Header)
	if err != nil {
		respondWithError(rWriter, 401, err.Error())
	}

	err = cfg.dbQueries.RevokeRefreshToken(rq.Context(), refreshToken)
	if err != nil {
		respondWithError(rWriter, 500, err.Error())
	}
	
	respondWithJSON(rWriter, 204, nil)
}

func (cfg *apiConfig) postChirpsHandler(rWriter http.ResponseWriter, rq *http.Request) {
	type parameters struct {
		Body 	string 		`json:"body"`
	}

	decoder := json.NewDecoder(rq.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(rWriter, 500, "error decoding parameters")
		return
	}

	if len(params.Body) > 140 {
		respondWithError(rWriter, 400, "chirp is too long")
		return
	}

	params.Body = profanityFilter(params.Body)

	jwtToken, err := auth.GetBearerToken(rq.Header)
	if err != nil {
		respondWithError(rWriter, 401, err.Error())
		return
	}

	authID, err := auth.ValidateJWT(jwtToken, cfg.jwtSecret)
	if err != nil{
		respondWithError(rWriter, 401, err.Error())
		return
	}

	chirp, err := cfg.dbQueries.CreateChirp(rq.Context(), database.CreateChirpParams{
		Body: params.Body,
		UserID: uuid.NullUUID{
			UUID: authID,
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

func (cfg *apiConfig) getMultipleChirpsHandler(rWriter http.ResponseWriter, rq *http.Request) {

	type returnVals struct {
		ID 			uuid.UUID	`json:"id"`
		Created_at 	time.Time 	`json:"created_at"`
		Updated_at	time.Time	`json:"updated_at"`
		Body		string		`json:"body"`
		User_id		uuid.UUID	`json:"user_id"`
	}

	authorID := rq.URL.Query().Get("author_id")
	sortParameter := rq.URL.Query().Get("sort")

	var chirps []database.Chirp
	var err error

	if authorID != "" {
		author_uuid, err := uuid.Parse(authorID)
		if err != nil {
			respondWithError(rWriter, 500, "error parsing author id")
			return
		}
		dbUUID := uuid.NullUUID{
			UUID: author_uuid,
			Valid: true,
		}
		chirps, err = cfg.dbQueries.GetChirpsFromAuthor(rq.Context(), dbUUID)
		if err != nil {
			respondWithError(rWriter, 500, "error retrieving chirps")
			return
		}
	} else {
		chirps, err = cfg.dbQueries.GetAllChirps(rq.Context())
		if err != nil {
			respondWithError(rWriter, 500, "error retrieving chirps")
			return
		}
	}

	var returnArr []returnVals

	for _, chirp := range chirps {
		respBody := returnVals{
			ID: 		chirp.ID,
			Created_at: chirp.CreatedAt,
			Updated_at: chirp.UpdatedAt,
			Body: 		chirp.Body,
			User_id: 	chirp.UserID.UUID,
		}
		returnArr = append(returnArr, respBody)
	}

	if sortParameter == "desc" {
		sortAlgorithm := func(i, j int) bool {
			idOne := returnArr[i].Created_at
			idTwo := returnArr[j].Created_at
			return idOne.String() > idTwo.String() 
		}
		sort.Slice(returnArr[:], sortAlgorithm)
	} else if sortParameter == "asc" || sortParameter == "" {
		sortAlgorithm := func(i, j int) bool {
			idOne := returnArr[i].Created_at
			idTwo := returnArr[j].Created_at
			return idOne.String() < idTwo.String() 
		}
		sort.Slice(returnArr[:], sortAlgorithm)
	}

	respondWithJSON(rWriter, 200, returnArr)
}

func (cfg *apiConfig) getOneChirpHandler(rWriter http.ResponseWriter, rq *http.Request) {
	
	id := rq.PathValue("chirpID")
	chirpId, err := uuid.Parse(id)
	if err != nil {
		respondWithError(rWriter, 500, "Error parsing chirp id")
		return
	}
	
	chirp, err := cfg.dbQueries.GetOneChirp(rq.Context(), chirpId)
	if err != nil {
		respondWithError(rWriter, 404, "No chirp found")
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

	respondWithJSON(rWriter, 200, respBody)
}

func (cfg *apiConfig) deleteOneChirpHandler(rWriter http.ResponseWriter, rq *http.Request) {
	

	jwtToken, err := auth.GetBearerToken(rq.Header)
	if err != nil {
		respondWithError(rWriter, 401, err.Error())
		return
	}

	authID, err := auth.ValidateJWT(jwtToken, cfg.jwtSecret)
	if err != nil{
		respondWithError(rWriter, 401, err.Error())
		return
	}

	id := rq.PathValue("chirpID")
	chirpId, err := uuid.Parse(id)
	if err != nil {
		respondWithError(rWriter, 500, "error parsing chirp id")
		return
	}

	chirp, err := cfg.dbQueries.GetOneChirp(rq.Context(), chirpId)
	if err != nil {
		respondWithError(rWriter, 404, "chirp not found")
		return
	}

	if chirp.UserID.UUID != authID {
		respondWithError(rWriter, 403, "unauthorized user")
		return
	}

	err = cfg.dbQueries.DeleteChirp(rq.Context(), chirp.ID)
	if err != nil {
		respondWithError(rWriter, 500, "error deleting chirp")
		return
	}

	respondWithJSON(rWriter, 204, nil)
}

func (cfg *apiConfig) usersPutHandler(rWriter http.ResponseWriter, rq *http.Request) {
	type parameters struct {
		Password 	string `json:"password"`
		Email 		string `json:"email"`
	}

	
	jwtToken, err := auth.GetBearerToken(rq.Header)
	if err != nil {
		respondWithError(rWriter, 401, err.Error())
		return
	}

	authID, err := auth.ValidateJWT(jwtToken, cfg.jwtSecret)
	if err != nil{
		respondWithError(rWriter, 401, err.Error())
		return
	}

	decoder := json.NewDecoder(rq.Body)
	rqParams := parameters{}
	err = decoder.Decode(&rqParams)
	if err != nil {
		respondWithError(rWriter, 500, "error decoding parameters")
		return
	}

	hashPass, err := auth.HashPassword(rqParams.Password)
	if err != nil {
		respondWithError(rWriter, 500, "error hashing password")
		return
	}

	dbParams := database.UpdatePasswordAndEmailParams{
		HashedPassword: hashPass,
		Email:			rqParams.Email,
		ID:				authID,
	}

	dbUser, err := cfg.dbQueries.UpdatePasswordAndEmail(rq.Context(), dbParams)
	if err != nil {
		respondWithError(rWriter, 500, "error updating password and email")
		return
	}

	type returnVals struct {
		Id 					uuid.UUID	`json:"id"`
		Created_at 			time.Time 	`json:"created_at"`
		Updated_at 			time.Time 	`json:"updated_at"`
		Email				string		`json:"email"`
		Is_chirpy_red		bool		`json:"is_chirpy_red"`
	}

	respBody := returnVals{
		Id:					dbUser.ID,
		Created_at: 		dbUser.CreatedAt,
		Updated_at: 		dbUser.UpdatedAt,
		Email:				dbUser.Email,
		Is_chirpy_red:		dbUser.IsChirpyRed.Bool,
	}
	
	respondWithJSON(rWriter, 200, respBody)
}


func (cfg *apiConfig) polkaWebhooksHandler(rWriter http.ResponseWriter, rq *http.Request) {
	type parameters struct {
		Event string `json:"event"`
		Data  struct {
			UserID string `json:"user_id"`
		} `json:"data"`
	}

	decoder := json.NewDecoder(rq.Body)
	rqParams := parameters{}
	err := decoder.Decode(&rqParams)
	if err != nil {
		respondWithError(rWriter, 500, "error decoding parameters")
		return
	}

	apiKey, err := auth.GetAPIKey(rq.Header)
	if err != nil {
		respondWithError(rWriter, 401, "no apiKey header found")
		return
	}

	cfgApiKey := cfg.polkaKey
	if apiKey != cfgApiKey {
		respondWithError(rWriter, 401, "incorrect api key")
		return
	}


	if rqParams.Event != "user.upgraded" {
		respondWithJSON(rWriter, 204, nil)
	} else {
		id, err := uuid.Parse(rqParams.Data.UserID)
		if err != nil {
			respondWithError(rWriter, 500, "error parsing user id")
			return
		}
		_, err = cfg.dbQueries.UpgradeUserToRed(rq.Context(), id)
		if err != nil {
			respondWithError(rWriter, 404, "error upgrading user")
			return
		}
	}


	respondWithJSON(rWriter, 204, nil)
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
	jwtSecret := os.Getenv("JWT_SECRET")
	polkaKey := os.Getenv("POLKA_KEY")

	apiCfg := &apiConfig{
		fileserverHits:	atomic.Int32{},
		dbQueries: 		dbQueries,
		platform:		platform,
		jwtSecret: 		jwtSecret,	
		polkaKey: 		polkaKey,	
	}
	serveHandler := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	serveMux.Handle("/app/", apiCfg.middlewareMetricsInc(serveHandler))

	healthHandler := func(rWriter http.ResponseWriter, rq *http.Request) {
		rWriter.Header().Add("Content-Type", "text/plain; charset=utf-8" )
		rWriter.WriteHeader(200)
		rWriter.Write([]byte("OK"))
	}
	serveMux.HandleFunc("GET /api/healthz", healthHandler)

	serveMux.HandleFunc("GET /api/chirps", apiCfg.getMultipleChirpsHandler)

	serveMux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.getOneChirpHandler)
	
	serveMux.HandleFunc("DELETE /api/chirps/{chirpID}", apiCfg.deleteOneChirpHandler)

	serveMux.HandleFunc("POST /api/chirps", apiCfg.postChirpsHandler)

	serveMux.HandleFunc("POST /api/users", apiCfg.usersHandler)

	serveMux.HandleFunc("PUT /api/users", apiCfg.usersPutHandler)

	serveMux.HandleFunc("POST /api/login", apiCfg.loginHandler)

	serveMux.HandleFunc("POST /api/refresh", apiCfg.refreshHandler)

	serveMux.HandleFunc("POST /api/revoke", apiCfg.revokeHandler)

	serveMux.HandleFunc("POST /api/polka/webhooks", apiCfg.polkaWebhooksHandler)

	serveMux.HandleFunc("GET /admin/metrics", apiCfg.metricsHandler)
	
	serveMux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)

	server := &http.Server{
		Handler:	serveMux,
		Addr: 		":8080",	
	}
	server.ListenAndServe()

}

func respondWithError(rWriter http.ResponseWriter, code int, msg string) {
	type errorStruct struct{
		Error string `json:"error"`
	}
	eStruct := errorStruct {
		Error: msg,
	}
	respondWithJSON(rWriter, code, eStruct)
}

func respondWithJSON(rWriter http.ResponseWriter, code int, payload interface{}) {
	if payload == nil {
		rWriter.WriteHeader(code)
		return
	}
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