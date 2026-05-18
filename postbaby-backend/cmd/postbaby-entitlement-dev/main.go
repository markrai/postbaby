package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"postbaby-backend/internal/store"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "grant":
		return runUpsert(args[1:], stdout, stderr, store.EntitlementStatusActive)
	case "revoke":
		return runUpsert(args[1:], stdout, stderr, store.EntitlementStatusCanceled)
	case "show":
		return runShow(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "error=unknown command: %s\n", args[0])
		printUsage(stderr)
		return 2
	}
}

type commandOptions struct {
	dbPath      string
	username    string
	entitlement string
}

func runUpsert(args []string, stdout, stderr io.Writer, status string) int {
	opts, err := parseCommandOptions(args, stderr)
	if err != nil {
		return 2
	}
	if err := validateDBPath(opts.dbPath, stderr); err != nil {
		return 1
	}

	sqliteStore, err := store.Open(opts.dbPath)
	if err != nil {
		fmt.Fprintf(stderr, "error=open database: %v\n", err)
		return 1
	}
	defer func() {
		_ = sqliteStore.Close()
	}()

	ctx := context.Background()
	user, err := sqliteStore.GetUserByUsername(ctx, opts.username)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			fmt.Fprintf(stderr, "error=user not found: %s\n", opts.username)
			return 1
		}
		fmt.Fprintf(stderr, "error=load user: %v\n", err)
		return 1
	}

	entitlement, err := sqliteStore.PutAccountEntitlement(
		ctx,
		user.ID,
		opts.entitlement,
		status,
		store.EntitlementSourceManual,
		nil,
	)
	if err != nil {
		fmt.Fprintf(stderr, "error=write entitlement: %v\n", err)
		return 1
	}

	printEntitlement(stdout, user.Username, entitlement)
	return 0
}

func runShow(args []string, stdout, stderr io.Writer) int {
	opts, err := parseCommandOptions(args, stderr)
	if err != nil {
		return 2
	}
	if err := validateDBPath(opts.dbPath, stderr); err != nil {
		return 1
	}

	sqliteStore, err := store.Open(opts.dbPath)
	if err != nil {
		fmt.Fprintf(stderr, "error=open database: %v\n", err)
		return 1
	}
	defer func() {
		_ = sqliteStore.Close()
	}()

	ctx := context.Background()
	user, err := sqliteStore.GetUserByUsername(ctx, opts.username)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			fmt.Fprintf(stderr, "error=user not found: %s\n", opts.username)
			return 1
		}
		fmt.Fprintf(stderr, "error=load user: %v\n", err)
		return 1
	}

	entitlement, err := sqliteStore.GetAccountEntitlement(ctx, user.ID, opts.entitlement)
	if err != nil {
		if errors.Is(err, store.ErrEntitlementNotFound) {
			printEntitlement(stdout, user.Username, store.AccountEntitlement{
				UserID:         user.ID,
				EntitlementKey: opts.entitlement,
				Status:         store.EntitlementStatusNone,
			})
			return 0
		}
		fmt.Fprintf(stderr, "error=read entitlement: %v\n", err)
		return 1
	}

	printEntitlement(stdout, user.Username, entitlement)
	return 0
}

func parseCommandOptions(args []string, stderr io.Writer) (commandOptions, error) {
	var opts commandOptions
	flags := flag.NewFlagSet("postbaby-entitlement-dev", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.dbPath, "db", "", "SQLite database path")
	flags.StringVar(&opts.username, "username", "", "Postbaby username")
	flags.StringVar(&opts.entitlement, "entitlement", store.EntitlementKeyHostedSync, "Entitlement key")
	if err := flags.Parse(args); err != nil {
		return commandOptions{}, err
	}

	if strings.TrimSpace(opts.dbPath) == "" {
		fmt.Fprintln(stderr, "error=missing required flag: --db")
		return commandOptions{}, errors.New("missing db path")
	}
	if strings.TrimSpace(opts.username) == "" {
		fmt.Fprintln(stderr, "error=missing required flag: --username")
		return commandOptions{}, errors.New("missing username")
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "error=unexpected arguments: %s\n", strings.Join(flags.Args(), " "))
		return commandOptions{}, errors.New("unexpected arguments")
	}
	if opts.entitlement != store.EntitlementKeyHostedSync {
		fmt.Fprintf(stderr, "error=unsupported entitlement: %s\n", opts.entitlement)
		return commandOptions{}, errors.New("unsupported entitlement")
	}

	return opts, nil
}

func printEntitlement(w io.Writer, username string, entitlement store.AccountEntitlement) {
	validUntil := ""
	if entitlement.ValidUntil != nil {
		validUntil = entitlement.ValidUntil.UTC().Format(timeLayout)
	}

	fmt.Fprintf(
		w,
		"username=%s entitlement=%s status=%s source=%s valid_until=%s\n",
		username,
		entitlement.EntitlementKey,
		entitlement.Status,
		entitlement.Source,
		validUntil,
	)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: postbaby-entitlement-dev <grant|revoke|show> --db PATH --username USERNAME [--entitlement hosted_sync]")
}

const timeLayout = "2006-01-02T15:04:05Z07:00"

func validateDBPath(dbPath string, stderr io.Writer) error {
	_, err := os.Stat(dbPath)
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(stderr, "error=database file does not exist: %s\n", dbPath)
		return err
	}

	fmt.Fprintf(stderr, "error=stat database file: %v\n", err)
	return err
}
