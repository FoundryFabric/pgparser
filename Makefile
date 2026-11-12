.PHONY: all build generate test clean check-parser cover cover-check

# PostgreSQL source location
PG_SRC := $(HOME)/Github/postgres

# goyacc pinned to the version that generated the committed parser/parser.go;
# another version lays the tables out differently.
GOYACC := go run golang.org/x/tools/cmd/goyacc@v0.40.0

all: generate build test

build:
	go build ./...

generate: generate-parser generate-nodes

generate-parser:
	cd parser && go generate

generate-nodes:
	go run ./cmd/pgsema-gen/... \
		-parsenodes $(PG_SRC)/src/include/nodes/parsenodes.h \
		-primnodes $(PG_SRC)/src/include/nodes/primnodes.h \
		-outdir nodes

test:
	go test ./...

# check-parser fails when regenerating parser/parser.go and parser/y.output
# from parser/gram.y, as go generate does (goyacc, then tools/yaccmaxdepth),
# does not reproduce the committed files byte for byte.
check-parser:
	@tmp=$$(mktemp -d) && cp parser/gram.y $$tmp/ && \
	(cd $$tmp && $(GOYACC) -o parser.go -p pg gram.y >/dev/null) && \
	go run ./tools/yaccmaxdepth -p pg $$tmp/parser.go && \
	if cmp -s parser/parser.go $$tmp/parser.go && cmp -s parser/y.output $$tmp/y.output; then \
		rm -rf $$tmp; echo "parser/parser.go matches parser/gram.y"; \
	else \
		diff -u parser/parser.go $$tmp/parser.go | head -20; rm -rf $$tmp; \
		echo "parser/parser.go is stale: run make generate-parser and commit it" >&2; exit 1; \
	fi

# cover measures the parser package's tests with parser/parser.go generated
# by goyacc -l, so that the grammar actions are seen: statements per file
# (go test -coverprofile), conditions per file (gobco) and the rules whose
# action no test reduces. It writes $(COVER_OUT)/head/summary.txt and
# facts.txt; see tools/parsercover.
COVER_OUT ?= .cover

cover:
	go run ./tools/parsercover run -src . -out $(COVER_OUT)/head

# cover-check BASE=<rev> fails for each statement block, condition outcome
# or rule action the working tree's tests do not reach that BASE's reached
# or did not have, matched by file, function or rule and normalised source
# text rather than by line or rule number.
cover-check:
	@test -n "$(BASE)" || { echo "usage: make cover-check BASE=<rev>" >&2; exit 2; }
	@tmp=$$(mktemp -d) && git archive $(BASE) | tar -x -C $$tmp && \
	go run ./tools/parsercover run -src $$tmp -out $(COVER_OUT)/base; s=$$?; rm -rf $$tmp; exit $$s
	go run ./tools/parsercover run -src . -out $(COVER_OUT)/head
	go run ./tools/parsercover check -base $(COVER_OUT)/base/facts.txt -head $(COVER_OUT)/head/facts.txt

clean:
	rm -f parser/y.go
	rm -f nodes/*_generated.go

# Install goyacc if not present
install-tools:
	go install golang.org/x/tools/cmd/goyacc@v0.40.0

# Copy PostgreSQL regression test SQL files
copy-regress-sql:
	mkdir -p parser/pgregress/testdata/sql
	cp $(PG_SRC)/src/test/regress/sql/*.sql parser/pgregress/testdata/sql/

# Update known_failures.json baseline
update-known-failures:
	cd parser/pgregress && go test -run TestPGRegress -update -v -count=1

# Analyze PostgreSQL grammar (for development)
analyze-gram:
	@echo "Grammar lines: $$(wc -l < $(PG_SRC)/src/backend/parser/gram.y)"
	@echo "makeNode calls: $$(grep -c 'makeNode(' $(PG_SRC)/src/backend/parser/gram.y)"
	@echo "list_make calls: $$(grep -c 'list_make' $(PG_SRC)/src/backend/parser/gram.y)"
	@echo "Keywords: $$(grep -c 'PG_KEYWORD' $(PG_SRC)/src/include/parser/kwlist.h)"
