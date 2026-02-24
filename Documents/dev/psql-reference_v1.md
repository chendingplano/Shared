# PostgreSQL psql Quick Reference Card

## Connection Commands

| Command/Flag | Description | Example |
|--------------|-------------|---------|
| `psql -d <db-name>` | Specifies the database name to connect to | `psql -d tutorials_db` |
| `psql -U <username>` | Specifies the user to connect as | `psql -U admin` |
| `psql -W` | Forces psql to prompt for password | `psql -W` |
| `psql -h <db-address>` | Specifies the host address of the database | `psql -h my-psql-db.cloud.neon.tech` |

### Connection Patterns

**Same Host:**
```bash
psql -d <db-name> -U <username> -W
```

**Different Host:**
```bash
psql -h <db-address> -d <db-name> -U <username> -W
```

**SSL Connection:**
```bash
psql "sslmode=require host=<db-address> dbname=<db-name> user=<username>"
```

---

## Database Information Commands

| Command | Description |
|---------|-------------|
| `\l` | List all databases |
| `\c <db-name>` | Switch to another database |
| `\dn` | List all schemas |

---

## Table & Structure Commands

| Command | Description |
|---------|-------------|
| `\dt` | List all database tables |
| `\d <table-name>` | Describe a table's structure (columns, types, nullability, defaults) |
| `\d+ <table-name>` | Describe table with extended info (storage, compression, stats, description) |

---

## User & Role Commands

| Command | Description |
|---------|-------------|
| `\du` | List all users and their roles |
| `\du <username>` | Retrieve specific user info (roles and group memberships) |

---

## Functions & Views Commands

| Command | Description |
|---------|-------------|
| `\df` | List all database functions |
| `\dv` | List all database views |

---

## File Operations Commands

| Command | Description |
|---------|-------------|
| `\o <file-name>` | Save query results to a file |
| `\o` (no args) | Stop saving to file, output to terminal |
| `\i <file-name>` | Run commands from a file |

---

## Exit Command

| Command | Description |
|---------|-------------|
| `\q` | Quit the psql interface |

---

## Quick Reference Summary

| Category | Commands |
|----------|----------|
| **Connection** | `-d`, `-U`, `-W`, `-h`, `sslmode` |
| **Database Info** | `\l`, `\c`, `\dn` |
| **Tables** | `\dt`, `\d`, `\d+` |
| **Users** | `\du`, `\du <username>` |
| **Objects** | `\df`, `\dv` |
| **Files** | `\o`, `\i` |
| **Exit** | `\q` |

---

## Important Notes

1. **Password Security**: The `-W` flag forces a password prompt, which is more secure than embedding passwords in connection strings.

2. **SSL Connections**: Use `sslmode=require` for secure connections to remote databases, especially cloud-hosted instances.

3. **Extended Table Info**: Use `\d+` instead of `\d` when you need detailed information about storage, compression, and statistics.

4. **File Output**: Remember to run `\o` without arguments to stop saving results to a file; otherwise, all output continues going to the file.

5. **Batch Commands**: The `\i` command is particularly useful for running complex SQL scripts or automating repetitive tasks.

6. **User Context**: When switching databases with `\c`, you remain logged in as the previously authenticated user.

---

*Source: https://hasura.io/blog/top-psql-commands-and-flags-you-need-to-know-postgresql*
