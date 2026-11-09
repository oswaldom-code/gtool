CREATE TABLE IF NOT EXISTS users (
    id   SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS products (
    id    SERIAL PRIMARY KEY,
    name  TEXT NOT NULL,
    price NUMERIC(10, 2) NOT NULL
);

INSERT INTO users (name) VALUES ('Ada Lovelace'), ('Alan Turing');
INSERT INTO products (name, price) VALUES ('Keyboard', 49.90), ('Monitor', 199.00);
