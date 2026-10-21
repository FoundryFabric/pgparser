-- Phase 2: core expression grammar gaps
SELECT TREAT('1' AS int);
-- expect-error: UNIQUE predicate is not yet implemented
SELECT UNIQUE (SELECT 1);
SELECT ROW();
SELECT 1 OPERATOR(pg_catalog.=) ANY (SELECT 1);
SELECT (SELECT 1)[1];
SELECT (SELECT 1).foo;
SELECT CAST('a' AS NATIONAL CHAR(5));
SELECT CAST('a' AS NCHAR(5));
SELECT SUBSTRING('abcdef', 2, 3);
SELECT OVERLAY('abcdef', 'Z', 3);
SELECT 33 * ANY ('{1,2,3}');
SELECT 33 * ANY (44);
-- Unary plus is an operator, as in PostgreSQL 16, wherever an expression may start
SELECT +1, + 2.5, +x, + +x, -+x, +(a AND b), a + +b, +a * b, +a::int, +'abc' FROM t;
SELECT x FROM t WHERE +x > +1 AND x BETWEEN +a AND +b AND +x IN (+1, +2);
INSERT INTO t (a, b) VALUES (+1, +x) RETURNING +a;
UPDATE t SET a = +a, b = +1 WHERE +a < +b;
SELECT 1 FROM t ORDER BY +a OFFSET +1 ROWS FETCH FIRST +2 ROWS ONLY;
SELECT f(+x, +1), ARRAY[+1, +x], (+1, +x), CASE WHEN +x = +1 THEN +a ELSE +b END FROM t;
