// LICENCE: No licence is provided for this project

package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"golang.org/x/net/publicsuffix"
)

const (
	droplistURL = "https://ausregistry.com.au/official-domain-name-drop-list/"
)

var (
	db *sqlx.DB
	cl *http.Client
)

func main() {
	var err error
	db, err = sqlx.Open("postgres", os.Getenv("SCANNER_DSN"))
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	cl = &http.Client{
		Timeout: 30 * time.Minute,
		Transport: &http.Transport{
			ResponseHeaderTimeout: 60 * time.Second,
			DisableKeepAlives:     false,
			MaxIdleConns:          10,
			IdleConnTimeout:       90 * time.Second,
		},
	}

	for {
		crawl()

		time.Sleep(5 * time.Minute)
	}

}

func crawl() {
	resp, err := cl.Get(droplistURL)
	if err != nil || resp.StatusCode != 200 {
		log.Printf("Droplist failed: %v/%v", err, resp)
		return
	}

	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Printf("Failed to create document: %v", err)
		return
	}

	m := map[string]struct{}{}
	doc.Find("table.domain-droplist td:nth-child(3)").Each(func(i int, s *goquery.Selection) {
		m[strings.ToLower(strings.TrimSpace(s.Text()))] = struct{}{}
	})

	log.Printf("Submitting %d names\n", len(m))
	submitNames(m)
}

func submitNames(domains map[string]struct{}) {
	now := time.Now().Unix()
	for name := range domains {
		etld, err := publicsuffix.EffectiveTLDPlusOne(name)
		if err != nil {
			log.Printf("Couldn't determine etld for %s: %v", name, err)
		}

		if _, err := db.Exec(`INSERT INTO domains (domain, first_seen, last_seen, etld) VALUES ($1, $2, $2, $3) ON CONFLICT (domain) DO NOTHING;`,
			name, now, etld); err != nil {
			log.Printf("Failed to insert/update %s: %v", name, err)
		}
	}
}
