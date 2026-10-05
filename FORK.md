# FoundryFabric fork of pgplex/pgparser

Branch `ff` is upstream `v0.2.0` plus the patches below. Tags are
`v0.2.0-ff.N`. The module path stays `github.com/pgplex/pgparser`, so a
consumer selects this fork with a `replace` directive and drops it once
upstream ships the fix.

| patch | file | pinned by |
|---|---|---|
| PARAM carries its `$N` number into the grammar's semantic value (every `ParamRef.Number` was 0) | `parser/parse.go` | `TestLexCarriesParamNumber` |
| Identifiers of 64+ bytes, quoted or not, are truncated to at most 63 bytes on a UTF-8 character boundary (pg_mbcliplen), as PostgreSQL 16's truncate_identifier does. `Parse` takes options: `WithNoticeHandler(func(Notice))` reports each truncation as `Notice{Code: "42622", Message: PostgreSQL's text, Location: token offset}` in source order (`Lexer.NoticeHandler` for direct lexer use); `WithStandardConformingStrings(bool)` sets the lexer setting (default on) | `parser/lexer.go`, `parser/parse.go` | `TestLexerTruncatesIdentifiers`, `TestParseNoticesInSourceOrder`, `TestParseStandardConformingStrings` |
