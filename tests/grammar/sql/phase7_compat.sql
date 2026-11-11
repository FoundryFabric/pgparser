-- Grammar compatibility fixtures for postgres features missing in pgparser.

-- expect-error: syntax error
ALTER DATABASE mydb REFRESH COLLATION VERSION;

-- expect-error: syntax error
ALTER TYPE mood DROP VALUE 'sad';

-- expect-error: syntax error
CREATE ASSERTION positive CHECK (1 = 1);
