package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
)

// runProbe reads one statement per line from stdin and prints PostgreSQL
// 16's verdict and the parser's side by side.
func runProbe(dsn string) error {
	ctx := context.Background()
	c, err := pgconn.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer c.Close(ctx)
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		sql := sc.Text()
		if sql == "" {
			continue
		}
		pg := result{Pos: -1}
		_, err := c.Exec(ctx, oraclePrefix+sql).ReadAll()
		var pe *pgconn.PgError
		switch {
		case errors.As(err, &pe) && pe.Code == "22012":
			pg.OK = true
		case errors.As(err, &pe):
			pg.Code, pg.Msg = pe.Code, pe.Message
			if pe.Position > 0 {
				pg.Pos = byteOffset(sql, int(pe.Position)-utf8.RuneCountInString(oraclePrefix))
			}
		default:
			return fmt.Errorf("%s: %v", sql, err)
		}
		fk := parseOne(0, sql)
		mark := "  "
		if k := disagreement(pg, fk); k != kindAgree {
			mark = "!!"
		}
		fmt.Printf("%s %s\n     pg16: %s\n     fork: %s\n", mark, sql, describe(pg), describe(fk))
	}
	return sc.Err()
}
