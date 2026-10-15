package main

import (
	"log"
	"net/http"
	"os"
	"time"

	api "bingo/internal/http"
	"bingo/internal/store"
	"bingo/web"
)

func main() {
	port := env("PORT", "8080")
	dbPath := env("DB_PATH", "bingo.db")

	st, err := store.New(dbPath)
	if err != nil {
		log.Fatalf("ouverture de la base %s : %v", dbPath, err)
	}
	defer st.Close()

	srv := api.New(st)
	handler := srv.Routes(http.FileServerFS(web.FS))

	httpSrv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	log.Printf("BINGO! en écoute sur http://localhost:%s (base : %s)", port, dbPath)
	log.Fatal(httpSrv.ListenAndServe())
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
