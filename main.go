package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/dtan4/ct2stimer/crontab"
	"github.com/dtan4/ct2stimer/systemd"
	flag "github.com/spf13/pflag"
)

var opts = struct {
	after    string
	filename string
	outdir   string
	reload   bool
}{}

func parseArgs(args []string) error {
	f := flag.NewFlagSet("ct2stimer", flag.ExitOnError)

	f.StringVar(&opts.filename, "after", "", "unit dependencies (After=)")
	f.StringVarP(&opts.filename, "file", "f", "", "crontab file")
	f.StringVarP(&opts.outdir, "outdir", "o", systemd.DefaultUnitsDirectory, "directory to save systemd files")
	f.BoolVar(&opts.reload, "reload", false, "reload & start genreated timers")

	f.Parse(args)

	if opts.filename == "" {
		return fmt.Errorf("Please specify crontab file.")
	}

	if opts.outdir == "" {
		return fmt.Errorf("Please specify directory to save systemd files.")
	}

	if _, err := os.Stat(opts.outdir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: directory does not exist", opts.outdir)
		}

		return err
	}

	return nil
}

func reloadSystemd(timers []string) error {
	conn, err := systemd.NewConn()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer conn.Close()

	client := systemd.NewClient(conn)

	if err := client.Reload(); err != nil {
		return err
	}

	for _, timerUnit := range timers {
		if err := client.StartUnit(timerUnit); err != nil {
			return err
		}
	}

	return nil
}

func main() {
	if err := parseArgs(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	body, err := ioutil.ReadFile(opts.filename)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	schedules, err := crontab.Parse(string(body))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	timers := []string{}

	for _, schedule := range schedules {
		calendar, err := schedule.ConvertToSystemdCalendar()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		name := "cron-" + schedule.SHA256Sum()[0:12]

		service, err := systemd.GenerateService(name, schedule.Command, opts.after)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		servicePath := filepath.Join(opts.outdir, name+".service")
		if err := ioutil.WriteFile(servicePath, []byte(service), 0644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		timer, err := systemd.GenerateTimer(name, calendar)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		timerPath := filepath.Join(opts.outdir, name+".timer")
		if err := ioutil.WriteFile(timerPath, []byte(timer), 0644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		timers = append(timers, name+".timer")
	}

	if opts.reload {
		if err := reloadSystemd(timers); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
