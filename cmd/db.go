package cmd

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq" // add pq to binary
	"github.com/pressly/goose"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"

	"github.com/Syncano/acme-proxy/app/version"
)

const usageCommands = `
Commands:
    create NAME [go|sql]   Create new migration
    up                     Migrate the DB to the most recent version available
    up-to VERSION          Migrate the DB to a specific VERSION
    down                   Roll back the version by 1
    down-to VERSION        Roll back to a specific VERSION
    redo                   Re-run the latest migration
    status                 Dump the migration status for the current DB
    version                Print the current version of the database
`

var migrationCmd = &cli.Command{
	Name:  "db",
	Usage: "Migrate acme proxy server database structure.",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "dir", Usage: "migration directory",
			EnvVars: []string{"MIGRATION_DIR"}, Value: "./migrations",
		},
	},
	Action: func(c *cli.Context) error {
		db, err := sql.Open("postgres",
			fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
				dbOptions.User, dbOptions.Password, c.String("db-host"), c.String("db-port"), dbOptions.Database),
		)
		if err != nil {
			return err
		}

		if !c.Args().Present() {
			fmt.Println(usageCommands)

			return nil
		}

		stdlog, _ := zap.NewStdLogAt(logger.Logger(), zap.InfoLevel)
		goose.SetLogger(stdlog)

		logg := logger.Logger()

		if c.Args().First() == "up" {
			logg.With(
				zap.String("version", App.Version),
				zap.String("gitsha", version.GitSHA),
				zap.Time("buildtime", App.Compiled),
			).Info("Migration starting")
		}

		return goose.Run(c.Args().First(), db, c.String("dir"), c.Args().Tail()...)
	},
}

func init() {
	App.Commands = append(App.Commands, migrationCmd)
}
