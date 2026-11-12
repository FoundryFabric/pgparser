# The parser against PostgreSQL 16.15's parser: triage

`TestSweepIsPostgreSQL16s` (`parser/pg16_sweep_test.go`) pins PostgreSQL
16.15's parser's verdict on every regression statement, on mutations of them
and on hand-written probes (`testdata/pg16_sweep.jsonl`). Each disagreement
the parser still has is tagged with one of the classes below. A fix for a
class drops the tag from its rows; the test fails for any tagged row the
parser now agrees on, and for any untagged row it disagrees on.

## How the verdicts were taken

`tools/pg16sweep` (its own module, so the root module needs neither pgx nor a
database) sends each statement S to PostgreSQL 16.15 as the simple query
`select 1/0;S`. PostgreSQL parses the whole query string before it runs any of
it, so `22012 division by zero` means its parser accepted S, and any other
error is its parser's (or its scanner's, or a grammar action's), at a position
within S once the prefix is taken off; the 1-based character position is
turned into the parser's 0-based byte offset. Nothing in S ever runs. Checked
first by hand: `select 1; select 2` and `select 1; selec 2` (a later statement
is parsed, and its error comes before any execution), `copy t from stdin`
(accepted; the data never reaches the parser), `select :foo` (refused at `:`,
which is why statements with psql variables are left out), multi-byte text
before the error (`select 'éé日本' + ;`, positions agree in bytes), and
`vacuum`, `begin`, `create table` (accepted; never run).

| corpus | statements |
|---|---|
| regression statements (`pgregress.ExtractStatements` over the 224 files) | 45,075 |
| left out: psql variables after extraction | 250 |
| left out: invalid UTF-8 | 4 |
| mutations, 2 per statement, fixed seed per file and statement | 90,150 |
| probes: `tools/pg16sweep/probes.sql` and the `tests/grammar/sql` fixtures | 157 |
| total sent to PostgreSQL 16.15 and to the parser | 135,382 |

The mutations: delete a token (11,411), duplicate one (11,441), swap one with
the next (9,464), insert `(` `)` `*` `,` `.` `.*` `[1]` `[1:2]` after one
(11,581 together), add a dotted part `.x` (11,526), a type modifier `(3)`
(11,528) or `[]` (11,550) after a word, preferring one after `::`, and replace
a word with a PostgreSQL 16 reserved keyword (11,649). The oracle took 2.4 s
(8 connections); the parser 1.3 s. No statement was left unclassified by the
oracle, and the database was left as it was.

The test data keeps every regression statement and probe, and of the
mutations every disagreement, every one PostgreSQL 16 accepts or refuses with
something other than a syntax error, and one in 40 of the rest: 58,864 rows,
3.5 MB.

## Totals

| | regression | mutations | probes |
|---|---|---|---|
| agree (branch `ff`) | 44,651 | 89,011 | 67 |
| parser accepts what PostgreSQL 16 refuses | 410 | 93 | 48 |
| parser refuses what PostgreSQL 16 accepts | 6 | 141 | 31 |
| both refuse, different SQLSTATE or message | 8 | 894 | 11 |
| both refuse, same message, different position | 0 | 11 | 0 |
| crash, panic or timeout | 0 | 0 | 0 |

No statement made the parser panic or crash, and none took more than 229 µs
to parse. With open PR #15 (`fix/typename-sites`, 1fdb80c) the parser
disagrees on 1,601 statements instead of 1,653; it fixes all of
`drop-type-typename` and `alter-extension-member` and makes no statement
disagree that agreed before.

## Classes, worst first

Risk 1 is wrong SQL accepted, 2 valid SQL refused, 3 a message or position
only. A class's statements can include mutations whose disagreement is a
message or position: a mutation of a statement that disagrees, refused by
PostgreSQL 16 where it refused the statement, has the statement's class; a
mutation the parser refuses earlier than PostgreSQL 16 does has the class of
what it refused. Size: S a rule or an action, M a few rules or a keyword list,
L a feature's grammar.

| class | risk | statements (regr/mut/probe) | direction | gram.y | #15 | size |
|---|---|---|---|---|---|---|
| pg17-sqljson | 1 | 714 (334/369/11) | accepts PostgreSQL 17's SQL/JSON | `func_expr_common_subexpr` JSON_QUERY, JSON_VALUE, JSON_EXISTS, JSON(), JSON_SCALAR, JSON_SERIALIZE; `json_table` in `table_ref`; `json_returning_clause_opt`, `json_behavior_clause_opt`, `json_wrapper_behavior`, `json_quotes_clause_opt`, `json_passing_clause_opt`, `json_table_column_definition` | no | L |
| pg17-merge | 1 | 58 (31/25/2) | accepts PostgreSQL 17's MERGE | `merge_when_clause` (`BY SOURCE`, `BY TARGET`), `MergeStmt` `returning_clause`, `MERGE_ACTION()` | no | M |
| pg17-ddl | 1 | 42 (22/15/5) | accepts PostgreSQL 17's DDL | `set_access_method_name` (DEFAULT), `alter_table_cmd` SET EXPRESSION, `a_expr AT LOCAL`, `DomainConstraintElem` NOT NULL in ALTER DOMAIN ADD, `operator_def_elem` without `=` | no | M |
| utility-option-argument (fixed) | 1 | 55 (0/52/3) | accepts `*`, DEFAULT and `(list)` as an EXPLAIN/VACUUM/ANALYZE option argument | `utility_option_arg` | no | S |
| publication-object-list (fixed) | 1 | 24 (6/15/3) | accepts what `preprocess_pubobj_list` refuses | `CreatePublicationStmt`, `AlterPublicationStmt`, `PublicationObjSpec` actions | no | S |
| grammar-action-checks (fixed) | 1 | 21 (8/6/7) | accepts what a grammar action refuses | `CopyStmt`, `parsePartitionStrategy`, `alter_table_cmd` column number, `CreateSchemaStmt`, `TriggerEvents`, `xmltable_column_option_el`, `frame_extent`, `json_encoding_clause_opt` | no | M |
| cte-column-list-parentheses (fixed) | 1 | 12 (0/10/2) | accepts a CTE's column names without parentheses | `opt_name_list` | no | S |
| constraint-attribute-checks (fixed) | 1 | 9 (5/1/3) | accepts what `processCASbits` and constraint actions refuse | `ConstraintAttributeSpec`/`processCASbits`, `key_action` SET NULL (cols), `ColConstraintElem` GENERATED | no | S |
| select-clause-checks (fixed) | 1 | 8 (3/2/3) | accepts WITH TIES without ORDER BY, with SKIP LOCKED; ORDER BY with WITHIN GROUP | `insertSelectOptions`, `func_application` within `func_expr` | no | S |
| uescape (fixed) | 1 | 7 (2/4/1) | accepts an invalid UESCAPE character; other message for a UESCAPE not followed by a string | scanner and `base_yylex` UESCAPE handling (`check_uescapechar`) | no | S |
| with-oids (fixed) | 1 | 5 (2/1/2) | accepts WITH OIDS | `OptWith`, `alter_table_cmd` SET WITH OIDS | no | S |
| drop-aggregate-arguments (fixed) | 1 | 4 (1/1/2) | accepts DROP AGGREGATE without argument types | `DropStmt` AGGREGATE `aggregate_with_argtypes_list` | no | S |
| partition-command-in-list (fixed) | 1 | 4 (0/2/2) | accepts ATTACH/DETACH PARTITION in a command list | `AlterTableStmt` `partition_cmd` | no | S |
| role-name-checks (fixed) | 1 | 4 (0/1/3) | accepts `ALTER ROLE CURRENT_USER RENAME`; 42601 for 42939 | `RoleId` action, `RenameStmt` | no | S |
| bare-column-label (fixed) | 2 | 511 (2/502/7) | refuses a keyword as a column label without AS | `target_el: a_expr BareColLabel`, `bare_label_keyword` | no | M |
| pg17-json-names | 2 | 75 (4/66/5) | refuses PostgreSQL 16's json, json_table and the like as names | `kwlist.h`: 20 PostgreSQL 17 keywords, JSON col_name; `JsonType` | no | M (with pg17-sqljson) |
| drop-type-typename (fixed) | 2 | 50 (0/47/3) | refuses `DROP TYPE t[]`, `DROP DOMAIN d(3)` | `DropStmt` TYPE/DOMAIN `type_name_list` | yes, all 50 | S |
| alter-column-optional-keyword (fixed) | 2 | 7 (0/5/2) | refuses `ALTER a SET (...)`, `ALTER a SET GENERATED` without COLUMN | `alter_table_cmd` `ALTER opt_column ColId` | no | S |
| alter-role-no-options (fixed) | 2 | 4 (0/2/2) | refuses `ALTER ROLE r` | `AlterRoleStmt` `opt_with AlterOptRoleList` | no | S |
| reindex-optional-name (fixed) | 2 | 4 (1/0/3) | refuses `REINDEX SYSTEM`, `REINDEX DATABASE` | `ReindexStmt` `opt_single_name` | no | S |
| role-spec-keywords (fixed) | 2 | 4 (0/1/3) | refuses a type_func_name keyword as a role | `RoleSpec: NonReservedWord` | no | S |
| copy-generic-options (fixed) | 2 | 4 (0/2/2) | refuses a reserved keyword as a COPY option name; accepts DEFAULT as its argument | `copy_generic_opt_elem: ColLabel copy_generic_opt_arg` | no | S |
| alter-database-refresh-collation (fixed) | 2 | 3 (0/0/3) | refuses `ALTER DATABASE x REFRESH COLLATION VERSION` | `AlterDatabaseRefreshCollStmt` | no | S |
| column-options-qualified-name (fixed) | 2 | 3 (0/1/2) | refuses `ALTER COLUMN a SET (x.y = 1)` | `alter_table_cmd` SET/RESET `reloptions` (`reloption_elem`) | no | S |
| alter-extension-member (fixed) | 2 | 2 (0/0/2) | refuses ALTER EXTENSION ADD CAST, ADD TRANSFORM | `AlterExtensionContentsStmt` | yes, both | S |
| role-option-error-order (fixed) | 3 | 7 (0/6/1) | syntax error at the next token for PostgreSQL's `unrecognized role option` | `CreateOptRoleElem`/`AlterOptRoleElem: IDENT` | no | S |
| operator-missing-argument-message (fixed) | 3 | 5 (2/1/2) | hint folded into the message | `oper_argtypes` | no | S |
| policy-option-message (fixed) | 3 | 3 (1/1/1) | hint used as the message | `RowSecurityDefaultPermissive` | no | S |
| not-yet-implemented-errors (fixed) | 3 | 2 (0/0/2) | 42601 for 0A000 | `a_expr UNIQUE select_with_parens`, `CreateAssertionStmt` | no | S |
| drop-routine-list-name (fixed) | 3 | 2 (0/1/1) | refused at `[` instead of at `]` | `function_with_argtypes_list` | no | S |

Totals: 1,653 statements; 30 classes. Every disagreement is in a class.

## The classes

### pg17-sqljson (risk 1, 714)

The parser takes PostgreSQL 17's SQL/JSON query functions, constructors and
JSON_TABLE. PostgreSQL 16 has `json_query(...)`, `json_value(...)`,
`json_exists(...)`, `json(...)`, `json_scalar(...)` and `json_serialize(...)`
only as ordinary function calls, so it refuses their RETURNING, PASSING, ON
ERROR, ON EMPTY, WRAPPER, QUOTES and FORMAT clauses at the first one, and has
no JSON_TABLE. The parser's own `CLAUDE.md` names PostgreSQL 17.7 as its
target; this class and the next four PostgreSQL 17 classes are what that
looks like to PostgreSQL 16. `tests/grammar/sql/phase5_plan.sql` pins five of
these statements (JSON_VALUE ... EMPTY ON ERROR, JSON_TABLE columns) as
parsing, and its expectations name `JsonTableColumn`; the fix changes them.

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `select json_value(j, '$' returning int)` | 42601 syntax error at or near "returning" at 25 | accepts |
| `select json_query(j, '$' with wrapper)` | 42601 syntax error at or near "with" at 25 | accepts |
| `select json_exists(j, '$' error on error)` | 42601 syntax error at or near "error" at 26 | accepts |
| `select * from json_table(j, '$' columns (a int))` | 42601 syntax error at or near "columns" at 32 | accepts |
| `select json('1' with unique keys)` | 42601 syntax error at or near "with" at 16 | accepts |

### pg17-merge (risk 1, 58)

MERGE's RETURNING, `merge_action()` and `WHEN NOT MATCHED BY SOURCE | BY
TARGET` are PostgreSQL 17's.

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `merge into t using s on t.a = s.a when matched then delete returning *` | 42601 syntax error at or near "returning" at 59 | accepts |
| `merge into t using s on t.a = s.a when not matched by source then delete` | 42601 syntax error at or near "by" at 51 | accepts |
| `MERGE INTO target t USING source AS s ON t.tid = s.sid WHEN NOT MATCHED BY SOURCE THEN INSERT DEFAULT VALUES` | 42601 syntax error at or near "BY" at 110 | 42601 syntax error at or near "INSERT" at 126 |

### pg17-ddl (risk 1, 42)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `alter table t set access method default` | 42601 syntax error at or near "default" at 32 | accepts |
| `alter table t alter c set expression as (1)` | 42601 syntax error at or near "expression" at 26 | accepts |
| `select now() at local` | 42601 syntax error at or near "local" at 16 | accepts |
| `alter domain d add not null` | 42601 syntax error at or near "not" at 19 | accepts |
| `alter operator === (int, int) set (merges)` | 42601 syntax error at or near ")" at 41 | accepts |

### utility-option-argument (risk 1, 55; fixed)

`utility_option_arg` has `'*'`, `DEFAULT` and `'(' utility_option_arg_list
')'`, which are COPY's (`copy_generic_opt_arg`); PostgreSQL 16's is
`opt_boolean_or_string | NumericOnly | /* EMPTY */`. Most of the 55 are
mutations such as `VACUUM (ANALYZE(3)) t`, refused one token later than
PostgreSQL refuses them.

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `explain (costs *) select 1` | 42601 syntax error at or near "*" at 15 | accepts |
| `explain (costs (a, b)) select 1` | 42601 syntax error at or near "(" at 15 | accepts |
| `explain (costs default) select 1` | 42601 syntax error at or near "default" at 15 | accepts |
| `VACUUM (ANALYZE(3)) vacowned_part2` | 42601 syntax error at or near "(" at 15 | 42601 syntax error at or near "3" at 16 |

`utility_option_arg` is now `opt_boolean_or_string | NumericOnly | /* EMPTY
*/`, and `utility_option_name` takes `analyze_keyword` and no longer DEFAULT.
COPY's parenthesised options have their own `copy_generic_opt_list`; all 55
agree, and so do the 4 of copy-generic-options.

### publication-object-list (risk 1, 24; fixed)

PostgreSQL 16's `preprocess_pubobj_list` refuses a list whose first object
has no TABLE or TABLES IN SCHEMA, CURRENT_SCHEMA as a table, a qualified
schema name, and a WHERE clause or column list on a schema.

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `create publication p for t` | 42601 invalid publication object list at 25 | accepts |
| `create publication p for current_schema` | 42601 invalid publication object list at 25 | accepts |
| `alter publication p add tables in schema s, s2 where (a = 1)` | 42601 WHERE clause not allowed for schema at 44 | accepts |
| `ALTER PUBLICATION testpub2 ADD testpub_tbl1 TABLE` | 42601 invalid publication object list at 31 | 42601 syntax error at or near "TABLE" at 44 |

`CreatePublicationStmt` and `AlterPublicationStmt` now check their object
list as `preprocess_pubobj_list` does, and the objects without a kind are
PostgreSQL 16's `ColId`, `ColId indirection` and `extended_relation_expr`
forms; all 24 agree.

### grammar-action-checks (risk 1, 21; fixed)

Errors PostgreSQL 16's grammar actions raise and the parser's do not, one
action each.

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `copy x to stdout where a = 1` | 42601 WHERE clause not allowed with COPY TO at 17 | accepts |
| `create table p (a int) partition by magic (a)` | 22023 unrecognized partitioning strategy "magic" | accepts |
| `alter index i alter column 0 set statistics 1` | 22023 column number must be in range from 1 to 32767 at 27 | accepts |
| `create schema if not exists s create table t (a int)` | 0A000 CREATE SCHEMA IF NOT EXISTS cannot include schema elements at 30 | accepts |
| `create trigger t before update or update of a on t for each row execute function f()` | 42601 duplicate trigger events specified at or near "on" at 46 | accepts |
| `select * from xmltable('/' passing x columns a int bogus 1)` | 42601 unrecognized column option "bogus" at 51 | accepts |
| `select json_object(returning text format json encoding bogus)` | 22023 unrecognized JSON encoding: bogus | accepts |

A window.sql mutation with BETWEEN deleted, `window w as (order by
f_interval range 'infinity'::interval following and ...)`, is in this class
too: PostgreSQL 16's `frame_extent` action refuses a lone FOLLOWING start
(42P20 frame starting from following row cannot end with current row, at
137); the parser has no such check and refuses at "and" (168).

Each of these actions now raises PostgreSQL 16's error: COPY's
STDIN/STDOUT with PROGRAM and WHERE with COPY TO, `parsePartitionStrategy`,
the column number of ALTER COLUMN n SET STATISTICS, CREATE SCHEMA IF NOT
EXISTS with elements, repeated `TriggerEvents`, the XMLTABLE column options
(an unknown or `__pg__is_not_null` name, a second PATH, DEFAULT or
NULL/NOT NULL), the JSON encoding and the five `frame_extent` checks; all 21
agree. A column number past 2147483647 is still refused as out of range
where PostgreSQL 16's scanner makes it a float and refuses it as a syntax
error.

### cte-column-list-parentheses (risk 1, 12; fixed)

The parser's `opt_name_list` is `name_list | /* EMPTY */`; PostgreSQL 16's is
`'(' name_list ')' | /* EMPTY */`, so the parser reads names after a CTE's
name as its column list. A mutation that turns a statement's first keyword
into WITH (`with ROLE regress_test_role`) shows it too.

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `with x a as (select 1) select 1` | 42601 syntax error at or near "a" at 7 | accepts |
| `with x a, b as (select 1) select 1` | 42601 syntax error at or near "a" at 7 | accepts |
| `with ROLE regress_test_role` | 42601 syntax error at or near "regress_test_role" at 10 | 42601 syntax error at end of input at 27 |

`opt_name_list` is now `'(' name_list ')' | /* EMPTY */`, and
`common_table_expr` takes it in one form; all 12 agree.

### constraint-attribute-checks (risk 1, 9; fixed)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `alter table t alter constraint c not deferrable initially deferred` | 42601 constraint declared INITIALLY DEFERRED must be DEFERRABLE at 48 | accepts |
| `ALTER TABLE fktable ALTER CONSTRAINT fktable_fk_fkey NOT VALID` | 0A000 FOREIGN KEY constraints cannot be marked NOT VALID | accepts |
| `ALTER TABLE fktable ALTER CONSTRAINT fktable_fk_fkey NO INHERIT` | 0A000 FOREIGN KEY constraints cannot be marked NO INHERIT | accepts |
| `create table t (a int references p on update set null (a))` | 0A000 a column list with SET NULL is only supported for ON DELETE actions at 35 | accepts |
| `create table t (a int generated by default as (1) stored)` | 42601 for a generated column, GENERATED ALWAYS must be specified at 32 | accepts |

Each of these actions now raises PostgreSQL 16's error: a conflicting or
INITIALLY DEFERRED but NOT DEFERRABLE `ConstraintAttributeSpec` at the
attribute that conflicts, `processCASbits` for each table and domain
constraint kind, ALTER CONSTRAINT (as FOREIGN KEY) and a constraint trigger,
a column list with SET NULL or SET DEFAULT in ON UPDATE at its ON, and
GENERATED BY DEFAULT on a generated column at BY; all 9 agree. A column's
REFERENCES takes only DEFERRABLE, NOT DEFERRABLE and INITIALLY, as
PostgreSQL 16's `ConstraintAttr` does, so NOT VALID and NO INHERIT there are
syntax errors.

### select-clause-checks (risk 1, 8; fixed)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `select 1 fetch first 1 row with ties` | 42601 WITH TIES cannot be specified without ORDER BY clause | accepts |
| `select 1 order by 1 fetch first 1 row with ties for update skip locked` | 42601 SKIP LOCKED and WITH TIES options cannot be used together | accepts |
| `select percentile_cont(p order by p) within group (order by x)` | 42601 cannot use multiple ORDER BY clauses with WITHIN GROUP at 37 | accepts |

`insertSelectOptions` now refuses WITH TIES without ORDER BY or with SKIP
LOCKED, and a second ORDER BY, OFFSET, LIMIT or WITH, and a limit after WITH
TIES, as PostgreSQL 16 does; `func_expr` refuses WITHIN GROUP after an
argument ORDER BY, DISTINCT or VARIADIC, at WITHIN. All 8 agree.

### uescape (risk 1, 7; fixed)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `select u&'x' uescape '+'` | 42601 invalid Unicode escape character at or near "'+'" at 21 | accepts |
| `SELECT U&' \' UESCAPE[] '!' AS "tricky"` | 42601 UESCAPE must be followed by a simple string literal at or near "[" at 21 | 42601 invalid Unicode escape at 11 |
| `SELECT 'tricky' AS U&"\" UESCAPE(3) '!'` | 42601 UESCAPE must be followed by a simple string literal at or near "(" at 32 | 42601 invalid Unicode escape at 22 |

A U& string or identifier is now decoded after the lexer has looked at the
next token, as PostgreSQL 16's `base_yylex` does. A UESCAPE there must be
followed by a quoted, escape or dollar-quoted string, or the statement is
`UESCAPE must be followed by a simple string literal` at that token (or at end
of input); the string must be one byte that is not a hex digit, `+`, a quote
or white space, or it is `invalid Unicode escape character` at the string.
Both come before an error in the U& literal's own escapes, and an error in
the next token comes before both. The fork scanned for the word UESCAPE and a
one-character string after it, and without them read UESCAPE as the next
token. All 7 agree, and so do the 5 `bare-column-label` rows left.

### with-oids (risk 1, 5; fixed)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `create table t () with oids` | 42601 syntax error at or near "oids" at 23 | accepts |
| `alter table t set with oids` | 42601 syntax error at or near "with" at 18 | accepts |

`create table t () without oids` is accepted by both.

`OptWith` no longer has `WITH OIDS`, and `alter_table_cmd` no longer has
`SET WITH OIDS`, as in PostgreSQL 16, which keeps only WITHOUT OIDS. All 5
agree.

### drop-aggregate-arguments (risk 1, 4; fixed)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `drop aggregate a` | 42601 syntax error at end of input at 16 | accepts |
| `drop aggregate if exists a, b` | 42601 syntax error at or near "," at 26 | accepts |
| `drop aggregate newcnt1` (errors.sql) | 42601 syntax error at end of input | accepts |

`DropStmt`'s own `DROP AGGREGATE any_name_list` forms are deleted, so DROP
AGGREGATE takes only `aggregate_with_argtypes_list`, as PostgreSQL 16's does.
All 4 agree.

### partition-command-in-list (risk 1, 4; fixed)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `alter table t attach partition p for values in (1), add column x int` | 42601 syntax error at or near "," at 50 | accepts |
| `alter table t detach partition p, add column x int` | 42601 syntax error at or near "," at 32 | accepts |

ATTACH PARTITION and DETACH PARTITION are now PostgreSQL 16's
`partition_cmd`, which only `ALTER TABLE [IF EXISTS] relation_expr` takes,
alone; they are no longer `alter_table_cmd`s, so they are refused in a
command list and after ALTER FOREIGN TABLE, VIEW, MATERIALIZED VIEW, SEQUENCE
and INDEX (which keeps its own `ATTACH PARTITION qualified_name`). All 4
agree.

### role-name-checks (risk 1, 4; fixed)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `alter role current_user rename to x` | 42939 CURRENT_USER cannot be used as a role name here at 11 | accepts |
| `create role current_user` | 42939 CURRENT_USER cannot be used as a role name here at 12 | 42601 role name cannot be a reserved keyword here at 12 |
| `create user current_role` | 42939 CURRENT_ROLE cannot be used as a role name here at 12 | 42601 role name cannot be a reserved keyword here at 12 |

`RoleId` now refuses PUBLIC (`42939 role name "public" is reserved`) and
CURRENT_ROLE, CURRENT_USER and SESSION_USER (`42939 <name> cannot be used as a
role name here`) at the role, and ALTER ROLE, ALTER USER and ALTER GROUP
RENAME TO take a `RoleId` on both sides (ALTER GROUP RENAME was missing); all
4 agree. The libpg_query tree comparison (`pg16sweep libpg`) found the cause
of the PUBLIC half: `RoleSpec` gave `ROLESPEC_CSTRING` "public" where
PostgreSQL 16 gives `ROLESPEC_PUBLIC`, for `public` in any case, quoted or
not, and it accepted `none`, which PostgreSQL refuses there (`42939 role name
"none" is reserved` at the role). Over the sweep corpus the
`RoleSpec.roletype` value classes (GrantStmt.grantees 76,
CreateUserMappingStmt.user 8, AlterUserMappingStmt.user 4,
CreatePolicyStmt.roles 4, DropUserMappingStmt.user 4) and the matching
`RoleSpec.rolename` name classes are gone. `RoleSpec` still has no location
(the `unlocated .../RoleSpec.location` classes). `TestRoleSpecsArePostgreSQL16s`
pins 1,555 role positions and spellings.

### bare-column-label (risk 2, 511; fixed)

The parser's `target_el` is `a_expr AS ColLabel | a_expr IDENT | a_expr`;
PostgreSQL 16's (since 14) takes `a_expr BareColLabel`, where a bare label is
an identifier or any keyword `bare_label_keyword` lists (432 of PostgreSQL
16's 471, reserved ones like CASE, IS, NOT and NULL included). The regression files use
it twice (`'regtype'::regtype type` in type_sanity.sql); most of the 511 are
mutations, and where PostgreSQL then refuses later, the parser refuses at the
label. The class was found by putting AS before the refused word: with it,
the parser gets past.

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `select 1 name` | accepts | 42601 syntax error at or near "name" at 9 |
| `select 1 interval` | accepts | 42601 syntax error at or near "interval" at 9 |
| `select 1 case` | accepts | 42601 syntax error at or near "case" at 9 |
| `select 1 is, 2` | accepts | 42601 syntax error at or near "," at 11 |
| `SELECT(3) time with time zone 'T0405+08'` | 42601 syntax error at or near "with" at 15 | 42601 syntax error at or near "time" at 10 |

`target_el` now takes `a_expr BareColLabel`, and 506 of the 511 agree. The
five left are `U&'...' UESCAPE` followed by something other than a string
(`+`, `.`, `[`, `(`): with UESCAPE a bare label the parser refuses at the
next token, PostgreSQL 16's position, but with `syntax error` where
PostgreSQL says `UESCAPE must be followed by a simple string literal`, the
message `uescape` is about. The `uescape` fix makes them agree.

### pg17-json-names (risk 2, 75)

The parser has PostgreSQL 17's 491 keywords, PostgreSQL 16 has 471. The
twenty more are `json_exists`, `json_query`, `json_scalar`, `json_serialize`,
`json_table`, `json_value`, `merge_action` (col_name) and `conditional`,
`empty`, `error`, `keep`, `nested`, `omit`, `path`, `plan`, `quotes`,
`source`, `string`, `target`, `unconditional` (unreserved), and `json` is a
col_name keyword with its own `JsonType` (no modifier, no qualification),
where PostgreSQL 16 has it unreserved. So PostgreSQL 16's function calls and
type names spelled with them are refused. Fixed with pg17-sqljson.

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `select 'x'::json(3)` | accepts | 42601 syntax error at or near "(" at 16 |
| `select 'x'::json.x` | accepts | 42601 syntax error at or near "." at 16 |
| `select json()` | accepts | 42601 syntax error at or near ")" at 12 |
| `select json_table(j)` | accepts | 42601 syntax error at or near "(" at 17 |
| `SELECT JSON_VALUE(3)(jsonb '[]', '$')` | 42601 syntax error at or near "(" at 20 | 42601 syntax error at or near ")" at 19 |

### drop-type-typename (risk 2, 50; fixed by #15)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `drop type t[]` | accepts | 42601 syntax error at or near "[" at 11 |
| `drop domain d(3)` | accepts | 42601 syntax error at or near "(" at 13 |
| `drop type t(1)[]` | accepts | 42601 syntax error at or near "(" at 11 |
| `DROP TYPE foo_t[1:2]` | 42601 syntax error at or near ":" at 17 | 42601 syntax error at or near "[" at 15 |

### alter-column-optional-keyword (risk 2, 7; fixed)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `alter table t alter a set (n_distinct = 1)` | accepts | 42601 syntax error at or near "(" at 26 |
| `alter table t alter a set generated by default` | accepts | 42601 syntax error at or near "generated" at 26 |
| `alter table parent alter a set(3) not null` | 42601 syntax error at or near "3" at 31 | 42601 syntax error at or near "(" at 30 |

`alter table t alter a set not null` is accepted by both.

`alter_table_cmd`'s ALTER forms are now PostgreSQL 16's `ALTER opt_column
ColId ...` (and `ALTER opt_column Iconst SET STATISTICS`), one rule each, so
COLUMN is optional in DROP EXPRESSION, DROP IDENTITY, the identity options
(SET GENERATED, RESTART, SET ...) and SET/RESET `(...)` too. The 16 rules
without COLUMN that duplicated others are deleted. All 7 agree.

### alter-role-no-options (risk 2, 4; fixed)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `alter role r` | accepts | 42601 syntax error at end of input at 12 |
| `alter user r` | accepts | 42601 syntax error at end of input at 12 |
| `ALTER ROLE REPLICATION` | accepts | 42601 syntax error at end of input at 23 |

`AlterRoleStmt` now has `ALTER ROLE|USER RoleSpec` with no options, which is
PostgreSQL 16's `opt_with AlterOptRoleList` with both empty; the tree is an
AlterRoleStmt with no options. All 4 agree.

### reindex-optional-name (risk 2, 4; fixed)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `reindex system` | accepts | 42601 syntax error at end of input at 14 |
| `reindex database` | accepts | 42601 syntax error at end of input at 16 |
| `REINDEX (CONCURRENTLY) SYSTEM` (create_index.sql) | accepts | 42601 syntax error at end of input |

REINDEX SYSTEM and REINDEX DATABASE now take PostgreSQL 16's
`opt_single_name`, so the name may be left out (`ReindexStmt.Name` is empty).
All 4 agree.

### role-spec-keywords (risk 2, 4; fixed)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `grant r to authorization` | accepts | 42601 syntax error at or near "authorization" at 11 |
| `create schema s authorization authorization` | accepts | 42601 syntax error at or near "authorization" at 30 |
| `alter group g add user verbose` | accepts | 42601 syntax error at or near "verbose" at 23 |

`RoleSpec` is now `NonReservedWord`, as PostgreSQL 16's is, so a
type_func_name keyword (`authorization`, `verbose`, `left` and the rest) is a
role wherever a role is taken. All 4 agree.

### copy-generic-options (risk 2, 4; fixed)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `copy t from stdin with (else csv)` | accepts | 42601 syntax error at or near "else" at 24 |
| `copy t from stdin with (default default)` | 42601 syntax error at or near "default" at 32 | accepts |

COPY's options are now `ColLabel copy_generic_opt_arg`, whose argument has
`'*'` and `'(' list ')'` but not DEFAULT; all 4 agree.

### alter-database-refresh-collation (risk 2, 3; fixed)

PostgreSQL 16 (since 15) parses `ALTER DATABASE name REFRESH COLLATION
VERSION`. `tests/grammar/sql/phase7_compat.sql` pins it as a syntax error,
as it does `ALTER EXTENSION hstore ADD CAST (text AS hstore)` and `ALTER
EXTENSION hstore ADD TRANSFORM FOR jsonb LANGUAGE plpgsql`
(alter-extension-member), which PostgreSQL 16 parses too; its other two,
`ALTER TYPE mood DROP VALUE 'sad'` and `CREATE ASSERTION`, PostgreSQL 16
refuses (CREATE ASSERTION with 0A000, not-yet-implemented-errors). The fix
changes those fixtures.

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `alter database x refresh collation version` | accepts | 42601 syntax error at or near "refresh" at 17 |
| `alter database x refresh` | 42601 syntax error at end of input at 24 | 42601 syntax error at or near "refresh" at 17 |

`AlterDatabaseRefreshCollStmt` (`ALTER DATABASE name REFRESH COLLATION
VERSION`) is new, with its node in `nodes`; the tree is libpg_query's.
`tests/grammar/sql/phase7_compat.sql` now expects it to parse. All 3 agree.

### column-options-qualified-name (risk 2, 3; fixed)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `alter table t alter column a set (x.y = 1)` | accepts | 42601 syntax error at or near "." at 35 |
| `alter table t alter column a reset (x.y)` | accepts | 42601 syntax error at or near "." at 37 |

`alter table t set (toast.autovacuum_enabled = false)` is accepted by both.

A column's SET and RESET now take `reloptions`, as PostgreSQL 16's do, so an
option name may be qualified (`x.y`), as a relation's may. All 3 agree.

### alter-extension-member (risk 2, 2; fixed by #15)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `ALTER EXTENSION hstore ADD CAST (text AS hstore)` | accepts | 42601 syntax error at or near "CAST" |
| `ALTER EXTENSION hstore ADD TRANSFORM FOR jsonb LANGUAGE plpgsql` | accepts | 42601 syntax error at or near "TRANSFORM" |

### role-option-error-order (risk 3, 7; fixed)

PostgreSQL raises `unrecognized role option` from the `IDENT` alternative's
action when it reduces it, before it looks at the next token; the parser
reports a syntax error at the next token first. Alone at the end
(`create role role x`) both give PostgreSQL's error.

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `create role role x with createrole` | 42601 unrecognized role option "x" at 17 | 42601 syntax error at or near "with" at 19 |
| `ALTER ROLE ROLE regress_hasprivs RENAME TO regress_tenant` | 42601 unrecognized role option "regress_hasprivs" at 16 | 42601 syntax error at or near "RENAME" at 33 |
| `CREATE USER MAPPING regress_addr_user SERVER "integer"` | 42601 unrecognized role option "regress_addr_user" at 21 | 42601 syntax error at or near "SERVER" at 39 |

The `IDENT` alternative now raises `unrecognized role option` through
`grammarError` and ends the parse when it is reduced, at the word, before
the parser looks at the next token; it set the error and went on, so a
syntax error at the next token replaced it. UNENCRYPTED PASSWORD, which had
the same fault, is now PostgreSQL 16's `0A000 UNENCRYPTED PASSWORD is no
longer supported` at UNENCRYPTED, with its hint (it was 42601 at the end of
the option). All 7 agree.

### operator-missing-argument-message (risk 3, 5; fixed)

PostgreSQL's message is `missing argument`, with HINT `Use NONE to denote the
missing argument of a unary operator.`; the parser's message is both.

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `drop operator === (int4)` | 42601 missing argument at 23 | 42601 missing argument, use NONE to denote the missing argument of a unary operator at 23 |

`oper_argtypes` now raises PostgreSQL 16's `missing argument` with its HINT
`Use NONE to denote the missing argument of a unary operator.` at the closing
parenthesis through `grammarError`, and ends the parse there, so a token
after it no longer replaces the error. All 5 agree.

### policy-option-message (risk 3, 3; fixed)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `create policy p on t as ugly using (true)` | 42601 unrecognized row security option "ugly" at 24 | 42601 only PERMISSIVE or RESTRICTIVE policies are supported at 24 |

`RowSecurityDefaultPermissive` now raises PostgreSQL 16's `unrecognized row
security option "<word>"` with HINT `Only PERMISSIVE or RESTRICTIVE policies
are supported currently.` at the word through `grammarError`, and ends the
parse there. All 3 agree.

### not-yet-implemented-errors (risk 3, 2; fixed)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `SELECT UNIQUE (SELECT 1)` | 0A000 UNIQUE predicate is not yet implemented (at UNIQUE) | 42601 UNIQUE predicate is not yet implemented (16 bytes later) |
| `CREATE ASSERTION positive CHECK (1 = 1)` | 0A000 CREATE ASSERTION is not yet implemented | 42601 syntax error at or near "ASSERTION" |

`a_expr UNIQUE opt_unique_null_treatment select_with_parens` now raises
`0A000 UNIQUE predicate is not yet implemented` at UNIQUE, and a new
production of `stmt` (`CREATE ASSERTION any_name CHECK '(' a_expr ')'
ConstraintAttributeSpec`) raises `0A000 CREATE ASSERTION is not yet
implemented` with no position, both through `grammarError`, ending the
parse. `tests/grammar/sql/phase7_compat.sql` now expects CREATE ASSERTION's
message. `SET CATALOG 'x'`, whose action had the same fault and made `Parse`
panic (a nil statement), is now `0A000 current database cannot be changed`
at the string. Both agree.

### drop-routine-list-name (risk 3, 2; fixed)

`drop procedure p[]` agrees; in a list, the parser refuses one token early.

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `drop procedure p, q[]` | 42601 syntax error at or near "]" at 20 | 42601 syntax error at or near "[" at 19 |

`DropStmt`'s own `DROP FUNCTION|PROCEDURE|ROUTINE any_name_list` forms are
deleted, so the list is `function_with_argtypes_list` from its first name, and
`q[]` is refused at `]`, as in PostgreSQL 16. Both agree.

## Rule coverage

`make cover` (`tools/parsercover`) lists the grammar rules whose action no
test reduces. On `ff` after #32 there were 253 of 2,491. Of the 244 that can
be reduced (goyacc reduces the other 9 in no state: no other rule uses
their nonterminals),
`TestRuleCoverageIsPostgreSQL16s` (`testdata/rule_coverage_pg16.jsonl`) pins
PostgreSQL 16.15's answers for at least one statement that reduces each,
taken by the oracle as above; where both accept, libpg_query 16's tree was
compared with the parser's (`pg16sweep libpg`).

9 of its statements are PostgreSQL 17 grammar that PostgreSQL 16 refuses.
They are tagged with a class, as the sweep's pg17 rows are, and
`checkPG16Answers` requires the parser to still differ from PostgreSQL 16 on
them:

| class | statements | rule |
|---|---|---|
| pg17-utility | `cluster (verbose)` | `ClusterStmt: CLUSTER '(' utility_option_list ')'`, with no table |
| pg17-utility | `copy t from stdin csv force not null *`, `copy t from stdin csv force null *` | `copy_opt_item: FORCE NOT NULL_P '*'`, `FORCE NULL_P '*'` |
| pg17-sqljson | `select json_query('1', '$' with conditional array wrapper)`, `... without array wrapper` | `json_wrapper_behavior` |
| pg17-sqljson | `select * from t, lateral json_table(...) j`, `select * from lateral json_table(...)` | `table_ref: LATERAL_P json_table opt_alias_clause` |
| pg17-ddl | `alter statistics s set statistics default`, with IF EXISTS | `set_statistics_value: DEFAULT` |

The statements' verdicts disagreed with PostgreSQL 16 in three more classes,
each fixed with its own test data below: `explain-create-table`,
`key-match-partial` and `grant-on-parameter`. Where both accepted, their
trees differed from libpg_query's in classes the regression corpus already
has (missing locations, `ObjectWithArgs.objfuncargs`, a list where
libpg_query has a `String`), except two, fixed below: `const-bit-typmods`
and `create-language-extension`.

### grant-on-parameter (risk 2, 1; fixed)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `grant alter system on parameter x to r` | accepts | 42601 syntax error at or near "x" at 32 |

The parser had no `ON PARAMETER` target for GRANT and REVOKE, so
`privilege: ALTER SYSTEM_P` could never be reduced. GRANT, REVOKE and
REVOKE GRANT OPTION FOR now take `PARAMETER parameter_name_list`, names as
SET takes them (a ColId, then any number of `.` ColId), each a `String` in
`Objects`, with `OBJECT_PARAMETER_ACL`. `testdata/grant_parameter_pg16.jsonl`
pins 500 statements; all agree, and where both accept, libpg_query's trees
differ only in the grantee's and grantor's location, which the parser does
not set anywhere (an `unlocated RoleSpec.location` class all over the
corpus).

### key-match-partial (risk 1, 1; fixed)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `create table t (a int references u match partial)` | 0A000 MATCH PARTIAL not yet implemented at 35 | accepts |

`key_match: MATCH PARTIAL` now raises `0A000 MATCH PARTIAL not yet
implemented` at MATCH through `grammarError`, ending the parse.
`testdata/match_partial_pg16.jsonl` pins 104 statements; all agree.

### explain-create-table (risk 1, 1; fixed)

| statement | PostgreSQL 16 | parser |
|---|---|---|
| `explain create table t (a int)` | 42601 syntax error at or near "int" at 26 | accepts |

The fork's `ExplainableStmt` had `CreateStmt`, which PostgreSQL 16's has
not; it is deleted, and EXPLAIN takes CREATE TABLE only through
`CreateAsStmt`. `testdata/explain_create_pg16.jsonl` pins 156 statements;
all agree.

### const-bit-typmods (tree; fixed)

| statement | libpg_query 16 | parser |
|---|---|---|
| `select bit '1'` | `TypeName` `bit`, no typmods | `TypeName` `bit`, typmods `[1]` |

`ConstBit: BitWithoutLength` kept `BitWithoutLength`'s BIT(1) modifier, which
is a column's default length; it now clears it. The 17 trees that changed
match libpg_query's. `testdata/const_bit_pg16.jsonl` pins 123 statements;
all agree.

## Rerunning

```
cd tools/pg16sweep
go build -o /tmp/pg16sweep .
/tmp/pg16sweep corpus -out corpus.jsonl
/tmp/pg16sweep oracle -corpus corpus.jsonl -out pg16.jsonl
/tmp/pg16sweep fork -corpus corpus.jsonl -out fork.jsonl
/tmp/pg16sweep compare -corpus corpus.jsonl -pg pg16.jsonl -fork fork.jsonl \
    -report report.txt -testdata ../../parser/testdata/pg16_sweep.jsonl
/tmp/pg16sweep probe < statements.sql   # PostgreSQL 16's and the parser's verdicts side by side
```

`-other` takes a second parser's results (another branch, built with its own
`replace`) to count what it fixes. The classes come from `classify.go`'s
rules; `report.txt` lists any disagreement no rule covers under class "".

## grammar conflicts

PostgreSQL 16's grammar builds with no conflicts. On `ff` before the fixes
below, gram.y had 10 shift/reduce and 23 reduce/reduce conflicts. goyacc
settles a shift/reduce conflict by shifting and a reduce/reduce conflict by
the rule that comes first in gram.y, so each conflict is a choice the parser
makes without a rule saying so. `TestGrammarConflictsArePinned`
(`parser/grammar_conflicts_test.go`) pins the counts in `parser/y.output`,
which `make check-parser` keeps goyacc's output for gram.y, so any change to
either count must edit the pin. `TestGrammarConflictsArePostgreSQL16s` pins
PostgreSQL 16.15's answers for the statements that reach each removed
conflict, one `testdata/grammar_conflict_*_pg16.jsonl` file for each cause.

The conflicts had six causes. Each had a fix that keeps PostgreSQL 16's
accept/reject, errors and trees, and none was left.

| cause | conflicts | where | what goyacc decided | wrong against PostgreSQL 16 | removed by |
|---|---|---|---|---|---|
| `substr_list` had its own `expr_list` form | 2 reduce/reduce, on `)` and `,` | after `SUBSTRING ( a_expr` | the function-argument form, so the extra form was never used | no | #26 |
| `DropStmt` duplicated `RemoveFuncStmt`, `RemoveAggrStmt`, `DropSubscriptionStmt` and `DropOwnedStmt` with any_name or name lists | 4 shift/reduce, 20 reduce/reduce | after a name in DROP FUNCTION, PROCEDURE, ROUTINE, AGGREGATE (with or without IF EXISTS), DROP SUBSCRIPTION and DROP OWNED BY | an unqualified function name as an any_name; DROP SUBSCRIPTION's name list before the end of input or `;`, but its single name before CASCADE or RESTRICT; DROP OWNED BY's RoleSpec | yes: `drop function f` was the list `[f]`, not an ObjectWithArgs; `drop subscription s, t` and `drop aggregate a` were accepted; `drop function f, s.g(int)` was refused at `(`; `drop procedure p, q[]` was refused at `[`, not `]` | #27 |
| ALTER COLUMN ADD GENERATED had four identity rules of its own beside `generated_when` | 2 shift/reduce, on AS | after `ALTER COLUMN c ADD GENERATED ALWAYS` or `BY DEFAULT` | the four rules, whose command had no `Def` | yes: no identity constraint in the tree; `as identity ()` was accepted | #28 |
| BEGIN and END were statements of a BEGIN ATOMIC body | 1 reduce/reduce, on `;` | after `BEGIN ATOMIC ... END` | the end of the body | yes: `begin atomic end transaction; end`, `begin atomic begin; end` and the like were accepted | #29 |
| a column's REFERENCES had its own list of DEFERRABLE, NOT DEFERRABLE and INITIALLY beside the column's `ConstraintAttr` | 3 shift/reduce, on DEFERRABLE, INITIALLY and NOT | after a column's `REFERENCES ...` | the REFERENCES list, folding the attributes into the foreign key | yes: the attributes are separate constraints in PostgreSQL 16's tree; `references x not null` was refused | #30 |
| NESTED in a JSON_TABLE column list could be a column name or start a nested path | 1 shift/reduce, on PATH | after NESTED | the nested path | no: PostgreSQL 16 has no JSON_TABLE (`pg17-sqljson`); the choice is kept, by precedence | #31 |
