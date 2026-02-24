# PSQL Quick Reference Card

> Source: https://hasura.io/blog/top-psql-commands-and-flags-you-need-to-know-postgresql

---

## Connection Commands

| Command/Flag | Description | Example |
|--------------|-------------|---------|
| `psql -d <db-name>` | Specifies the database name to connect to | `psql -d tutorials_db -U admin -W` |
| `psql -U <username>` | Specifies the user to connect as | `psql -d tutorials_db -U admin -W` |
| `psql -W` | Forces psql to prompt for password before connecting | `psql -d tutorials_db -U admin -W` |
| `psql -h <db-address>` | Specifies the host address of the database (for remote connections) | `psql -h my-psql-db.cloud.neon.tech -d tutorials_db -U admin -W` |
| `psql "sslmode=require host=<db-address> dbname=<db-name> user=<username>"` | Opens an SSL connection to the specified database | `psql "sslmode=require host=my-psql-db.cloud.neon.tech dbname=tutorials_db user=admin"` |

---

## Listing Commands

| Command | Description | Example |
|---------|-------------|---------|
| `\l` | Lists all available databases with name, owner, access privileges, and other information | `\l` |
| `\dt` | Lists all database tables with schema, type, and owner | `\dt` |
| `\dn` | Lists all database schemas with their names and owners | `\dn` |
| `\du` | Lists all users and their roles | `\du` |
| `\du <username>` | Retrieves information about a specific user including roles and group memberships | `\du postgres` |
| `\df` | Lists all functions with schema, names, result data type, argument data types, and type | `\df` |
| `\dv` | Lists all database views | `\dv` |

---

## Table Operations

| Command | Description | Example |
|---------|-------------|---------|
| `\d <table-name>` | Describes a table's structure including columns, types, nullability, and default values | `\d tutorials` |
| `\d+ <table-name>` | Describes a table with extended information (storage, compression, stats target, description) | `\d+ tutorials` |

---

## Database Navigation

| Command | Description | Example |
|---------|-------------|---------|
| `\c <db-name>` | Switches to another database under the previously logged-in user | `\c tutorials_db` |

---

## Query & File Operations

| Command | Description | Example |
|---------|-------------|---------|
| `\o <file-name>` | Saves query results to a file for later analysis or comparison | `\o query_results` |
| `\o` (without filename) | Stops saving results to file and outputs to terminal again | `\o` |
| `\i <file-name>` | Runs commands from a file (useful for multiple commands and complex SQL statements) | `\i psql_commands.txt` |

---

## Exit Command

| Command | Description | Example |
|---------|-------------|---------|
| `\q` | Quits the psql interface | `\q` |

---

## Summary by Category

| Category | Commands |
|----------|----------|
| **Connection** | `-d`, `-U`, `-W`, `-h`, `sslmode=require` |
| **Listing** | `\l`, `\dt`, `\dn`, `\du`, `\df`, `\dv` |
| **Table Operations** | `\d`, `\d+` |
| **Navigation** | `\c` |
| **File Operations** | `\o`, `\i` |
| **Exit** | `\q` |
