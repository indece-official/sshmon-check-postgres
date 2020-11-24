package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/miekg/dns"

	"database/sql"
	"database/sql/driver"

	_ "github.com/lib/pq"
)

// Variables set during build
var (
	ProjectName  string
	BuildVersion string
	BuildDate    string
)

var statusMap = []string{
	"OK",
	"WARN",
	"CRIT",
	"UNKNOWN",
}

var (
	flagVersion          = flag.Bool("v", false, "Print the version info and exit")
	flagService          = flag.String("service", "", "Service name (defaults to Elasticsearch_<host>)")
	flagHost             = flag.String("host", "", "Host")
	flagPort             = flag.Int("port", 5432, "Port")
	flagDatabase         = flag.String("db", "", "Database")
	flagUser             = flag.String("user", "", "User")
	flagPassword         = flag.String("password", "", "Password")
	flagPasswordFile     = flag.String("passwordfile", "", "File to read password from")
	flagDNS              = flag.String("dns", "", "Use alternate dns server")
	flagConnTimeout      = flag.Int("conntimeout", 5, "Connection timeout")
	flagMaxLockAge       = flag.Int("maxlockage", 0, "Maximum lock age in seconds")
	flagMaxQueryDuration = flag.Int("maxqueryduration", 0, "Maximum query duration in seconds")
)

// From https://stackoverflow.com/a/33678050

// Duration lets us convert between a bigint in Postgres and time.Duration
// in Go
type Duration time.Duration

// Value converts Duration to a primitive value ready to written to a database.
func (d Duration) Value() (driver.Value, error) {
	return driver.Value(int64(d)), nil
}

// Scan reads a Duration value from database driver type.
func (d *Duration) Scan(raw interface{}) error {
	switch v := raw.(type) {
	case int64:
		*d = Duration(v)
	case nil:
		*d = Duration(0)
	default:
		return fmt.Errorf("cannot sql.Scan() strfmt.Duration from: %#v", v)
	}
	return nil
}

func resolveDNS(host string) (string, error) {
	c := dns.Client{}
	m := dns.Msg{}

	m.SetQuestion(host+".", dns.TypeA)

	r, _, err := c.Exchange(&m, *flagDNS)
	if err != nil {
		return "", fmt.Errorf("Can't resolve '%s' on %s: %s", host, *flagDNS, err)
	}

	if len(r.Answer) == 0 {
		return "", fmt.Errorf("Can't resolve '%s' on %s: No results", host, *flagDNS)
	}

	aRecord := r.Answer[0].(*dns.A)

	return aRecord.A.String(), nil
}

func checkConnection(db *sql.DB) error {
	value := ""

	err := db.QueryRow(`
		SELECT 'test' AS value`,
	).Scan(&value)
	if err != nil {
		return err
	}

	if value != "test" {
		return fmt.Errorf("Postgres is crazy (returned '%s' instead of 'test')", value)
	}

	return nil
}

func checkLocks(db *sql.DB, maxLockAge int) (int, error) {
	countExceeded := 0

	err := db.QueryRow(`
		SELECT
			COUNT(*)
		FROM pg_catalog.pg_locks blockedl
		INNER JOIN pg_stat_activity blockeda
			ON blockedl.pid = blockeda.pid
		WHERE
			(now() - blockeda.query_start) > $1`,
		fmt.Sprintf("%d seconds", maxLockAge),
	).Scan(&countExceeded)
	if err != nil {
		return 0, err
	}

	return countExceeded, nil
}

func checkQueries(db *sql.DB, maxDuration int) (int, error) {
	countExceeded := 0

	err := db.QueryRow(`
		SELECT
			COUNT(*) as count
		FROM pg_stat_activity
		WHERE
			(now() - pg_stat_activity.query_start) > $1`,
		fmt.Sprintf("%d seconds", maxDuration),
	).Scan(&countExceeded)
	if err != nil {
		return 0, err
	}

	return countExceeded, nil
}

func main() {
	var err error

	flag.Parse()

	if *flagVersion {
		fmt.Printf("%s %s (Build %s)\n", ProjectName, BuildVersion, BuildDate)
		fmt.Printf("\n")
		fmt.Printf("https://github.com/indece-official/sshmon-check-postgres\n")
		fmt.Printf("\n")
		fmt.Printf("Copyright 2020 by indece UG (haftungsbeschränkt)\n")

		os.Exit(0)

		return
	}

	serviceName := *flagService
	if serviceName == "" {
		serviceName = fmt.Sprintf("Postgres_%s", *flagHost)
	}

	host := *flagHost

	if *flagDNS != "" {
		host, err = resolveDNS(host)
		if err != nil {
			fmt.Printf(
				"2 %s - %s - Error resolving ip of %s via dns %s: %s\n",
				serviceName,
				statusMap[2],
				*flagHost,
				*flagDNS,
				err,
			)

			os.Exit(0)

			return
		}
	}

	password := *flagPassword

	if *flagPasswordFile != "" {
		passwordBytes, err := ioutil.ReadFile(*flagPasswordFile)
		if err != nil {
			fmt.Printf(
				"2 %s - %s - Error reading password file %s: %s\n",
				serviceName,
				statusMap[2],
				*flagPasswordFile,
				err,
			)

			os.Exit(1)

			return
		}

		password = string(passwordBytes)
	}

	connStr := fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s connect_timeout=%d",
		host,
		*flagPort,
		*flagDatabase,
		*flagUser,
		password,
		*flagConnTimeout,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		fmt.Printf(
			"2 %s - %s - Error connecting to postgres database '%s' on %s:%d using user '%s': %s\n",
			serviceName,
			statusMap[2],
			*flagDatabase,
			host,
			*flagPort,
			*flagUser,
			err,
		)

		os.Exit(0)

		return
	}
	defer db.Close()

	err = checkConnection(db)
	if err != nil {
		fmt.Printf(
			"2 %s - %s - Error testing connection on database '%s' on %s:%d: %s\n",
			serviceName,
			statusMap[2],
			*flagDatabase,
			host,
			*flagPort,
			err,
		)

		os.Exit(0)

		return
	}

	if *flagMaxLockAge > 0 {
		countExceeded, err := checkLocks(db, *flagMaxLockAge)
		if err != nil {
			fmt.Printf(
				"2 %s - %s - Error loading active locks for database '%s' on %s:%d: %s\n",
				serviceName,
				statusMap[2],
				*flagDatabase,
				host,
				*flagPort,
				err,
			)

			os.Exit(0)

			return
		}

		if countExceeded > 0 {
			fmt.Printf(
				"1 %s - %s - %d locks on database '%s' on %s:%d have exceeded the max age of %d seconds\n",
				serviceName,
				statusMap[1],
				countExceeded,
				*flagDatabase,
				host,
				*flagPort,
				*flagMaxLockAge,
			)

			os.Exit(0)

			return
		}
	}

	if *flagMaxQueryDuration > 0 {
		countExceeded, err := checkQueries(db, *flagMaxQueryDuration)
		if err != nil {
			fmt.Printf(
				"2 %s - %s - Error loading long runnign queries for database '%s' on %s:%d: %s\n",
				serviceName,
				statusMap[2],
				*flagDatabase,
				host,
				*flagPort,
				err,
			)

			os.Exit(0)

			return
		}

		if countExceeded > 0 {
			fmt.Printf(
				"1 %s - %s - %d queries on database '%s' on %s:%d have exceeded the max duration of %d seconds\n",
				serviceName,
				statusMap[1],
				countExceeded,
				*flagDatabase,
				host,
				*flagPort,
				*flagMaxQueryDuration,
			)

			os.Exit(0)

			return
		}
	}

	fmt.Printf(
		"0 %s - %s - Postgres database '%s' on %s:%d is up and running\n",
		serviceName,
		statusMap[0],
		*flagDatabase,
		host,
		*flagPort,
	)

	os.Exit(0)
}
