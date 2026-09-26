# FoundryFabric fork of pgplex/pgparser

Branch `ff` is upstream `v0.2.0` plus the patches below. Tags are
`v0.2.0-ff.N`. The module path stays `github.com/pgplex/pgparser`, so a
consumer selects this fork with a `replace` directive and drops it once
upstream ships the fix.

| patch | file | pinned by |
|---|---|---|
| PARAM carries its `$N` number into the grammar's semantic value (every `ParamRef.Number` was 0) | `parser/parse.go` | `TestLexCarriesParamNumber` |
