# FoundryFabric fork of pgplex/pgparser

Branch `ff` is upstream `v0.2.0` plus the patches below. Tags are
`v0.2.0-ff.N`. The module path stays `github.com/pgplex/pgparser`, so a
consumer selects this fork with a `replace` directive and drops it once
upstream ships the fix.

| patch | file | pinned by |
|---|---|---|
| PARAM carries its `$N` number into the grammar's semantic value (every `ParamRef.Number` was 0) | `parser/parse.go` | `TestLexCarriesParamNumber` |
| Identifiers of 64+ bytes, quoted or not, are truncated to at most 63 bytes on a UTF-8 character boundary (pg_mbcliplen), as PostgreSQL 16's truncate_identifier does. `Parse(input, opts ...Option)` takes one option, `WithNoticeHandler(func(Notice))`, which reports each truncation in source order as `Notice{Code: "42622", Message: PostgreSQL's text}` (no position, as PostgreSQL sends none) | `parser/lexer.go`, `parser/parse.go` | `TestLexerTruncatesIdentifiers`, `TestParseNoticesInSourceOrder` |
| Unquoted identifiers have only ASCII `A`-`Z` lowercased, and keyword lookup folds ASCII case only, as PostgreSQL 16's downcase_identifier and ScanKeywordLookup do in a multibyte encoding (`Éa` stays `Éa`, `İx` stays `İx`; KELVIN SIGN + `ey` is an identifier, not `KEY`). `strings.ToLower` lowercased all Unicode and could change byte length, so truncation was computed on the wrong form | `parser/lexer.go`, `parser/keywords.go`, `cmd/pgsema-gen/main.go` | `TestParseDowncasesASCIIOnly`, `TestParseNonASCIIIsNeverAKeyword`, `TestParseTruncatesDowncasedForm` |
