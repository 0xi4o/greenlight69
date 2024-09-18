package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"github.com/joho/godotenv"
	"greenlight.i4o.dev/internal/data"
	"greenlight.i4o.dev/internal/mailer"
	"os"
	"strconv"
	"time"

	"github.com/charmbracelet/log"

	_ "github.com/lib/pq"
)

const version = "1.0.0"

type config struct {
	env  string
	port int
	db   struct {
		dsn          string
		maxOpenConns int
		maxIdleConns int
		maxIdleTime  time.Duration
	}
	limiter struct {
		rps     float64
		burst   int
		enabled bool
	}
	smtp struct {
		host     string
		port     int
		username string
		password string
		sender   string
	}
}

type application struct {
	config config
	logger *log.Logger
	models data.Models
	mailer mailer.Mailer
}

func main() {
	var cfg config

	logger := log.NewWithOptions(os.Stdout, log.Options{ReportCaller: true, ReportTimestamp: true})

	err := godotenv.Load()
	if err != nil {
		logger.Error("error reading .env", "err", err.Error())
		os.Exit(1)
	}

	var (
		dbName   = os.Getenv("DB_NAME")
		username = os.Getenv("DB_USERNAME")
		password = os.Getenv("DB_PASSWORD")
		dbHost   = os.Getenv("DB_HOST")
	)

	cfg.smtp.host = os.Getenv("SMTP_HOST")
	cfg.smtp.port, _ = strconv.Atoi(os.Getenv("SMTP_PORT"))
	cfg.smtp.username = os.Getenv("SMTP_USERNAME")
	cfg.smtp.password = os.Getenv("SMTP_PASSWORD")
	cfg.smtp.sender = os.Getenv("SMTP_SENDER")

	cfg.db.dsn = fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", username, password, dbHost, dbName)

	logger.Info("postgres dsn", "dsn", cfg.db.dsn)

	flag.IntVar(&cfg.port, "port", 4000, "API Server Port")
	flag.StringVar(&cfg.env, "env", "development", "Environment (development|staging|production)")
	flag.IntVar(&cfg.db.maxOpenConns, "db-max-open-conns", 25, "PostgreSQL maximum open connections")
	flag.IntVar(&cfg.db.maxIdleConns, "db-max-idle-conns", 25, "PostgreSQL maximum idle connections")
	flag.DurationVar(&cfg.db.maxIdleTime, "db-max-idle-time", 15*time.Minute, "PostgreSQL maximum connection idle time")

	flag.Float64Var(&cfg.limiter.rps, "limiter-rps", 2, "Rate limiter maximum requests per second")
	flag.IntVar(&cfg.limiter.burst, "limiter-burst", 4, "Rate limiter maximum burst")
	flag.BoolVar(&cfg.limiter.enabled, "limiter-enabled", true, "Enable rate limiter")

	flag.Parse()

	db, err := openDB(cfg)
	if err != nil {
		logger.Error("error opening db connection", "err", err.Error())
		os.Exit(1)
	}
	defer db.Close()

	logger.Info("database connection pool established")

	app := &application{
		config: cfg,
		logger: logger,
		mailer: mailer.New(cfg.smtp.host, cfg.smtp.port, cfg.smtp.username, cfg.smtp.password, cfg.smtp.sender),
		models: data.NewModels(db),
	}

	err = app.serve()
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
}

func openDB(cfg config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.db.dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(cfg.db.maxOpenConns)
	db.SetMaxIdleConns(cfg.db.maxIdleConns)
	db.SetConnMaxIdleTime(cfg.db.maxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
