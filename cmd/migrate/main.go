package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"

	dbmigrations "go-commerce/db"
	"go-commerce/pkg/serviceutil"
)

const defaultDSN = "root:password@tcp(127.0.0.1:3307)/ecommerce?charset=utf8mb4&parseTime=True&loc=Local"

func main() {
	log.SetFlags(0)
	if len(os.Args) != 2 {
		usage()
		os.Exit(2)
	}

	db, err := openDB(serviceutil.Env("DB_DSN", defaultDSN))
	if err != nil {
		log.Fatalf("mysql_connect_failed error=%v", err)
	}
	defer db.Close()

	if err := dbmigrations.Configure(); err != nil {
		log.Fatalf("migration_config_failed error=%v", err)
	}

	switch os.Args[1] {
	case "up":
		err = goose.Up(db, dbmigrations.Dir)
	case "down":
		err = goose.Down(db, dbmigrations.Dir)
	case "status":
		err = goose.Status(db, dbmigrations.Dir)
	case "version":
		err = printVersion(db)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatalf("migration_%s_failed error=%v", os.Args[1], err)
	}
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func printVersion(db *sql.DB) error {
	version, err := goose.GetDBVersion(db)
	if err != nil {
		return err
	}
	fmt.Println(version)
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: go run ./cmd/migrate {up|down|status|version}")
}
