# tip 🧰

[![nightly-build](https://github.com/c4pt0r/tip/actions/workflows/ci.yml/badge.svg)](https://github.com/c4pt0r/tip/actions/workflows/ci.yml)

`tip` is the Swiss Knife for interacting with TiDB databases (especially for TiDB Serverless) in your shell workflow. It provides a user-friendly way to connect to TiDB servers, execute queries, and view results in various formats.

A quick look 👀：

```
echo 'table1\ntable2\ntable3' | parallel \
'./tip -o json -e "SELECT COUNT(*) as count FROM {}" | jq -r ".[] | \"\(.count) records in {} table\""'
```

## Installation

Install:

```
curl -fsSL https://raw.githubusercontent.com/c4pt0r/tip/refs/heads/master/install.sh | sh
```

Configuration:

```
vim ~/.tip/config.toml
# More details in Configuration part 
```

Try it:
```
export PATH=$PATH:~/.tip/bin
tip -host 127.0.0.1 -p 4000 -u root -P "" -d test -e "select tidb_version();" -o json
```


## Usage

Basic usage:

```
tip [flags]
```

Flags:

- `-host`: TiDB Serverless hostname
- `-port`: TiDB port
- `-u`: TiDB username
- `-p`: TiDB password
- `-d`: TiDB database
- `-c`: Path to configuration file (default: `~/.tip/config.toml`)
- `-o`: Output format: plain, table (default), or json
- `-e`: Execute SQL statement and exit
- `-v`: Display execution details
- `-version`: Display version information

Example:

```
tip -host mytidbserver.com -port 4000 -u myuser -p mypassword -d mydatabase
```

### Interactive Commands

Once connected, you can use the following commands in the interactive shell:

- `.help` - Display help information
- `.ver` - Display version information
- `.connect <host> <port> <user> <password> [database]` - Connect to a database
- `.output_format [format]` - Set or display output format (json/table/plain/csv)
- `.lua-eval "<script>" [args...]` - Execute Lua script with SQL integration

### Lua Integration

The `.lua-eval` command allows you to execute Lua scripts with direct SQL integration. It provides two main functions:

- `sql.query(query)` - Execute a SELECT query and return results as a table
- `sql.execute(query)` - Execute an INSERT/UPDATE/DELETE query and return affected rows

You can pass arguments to your Lua script after the script string. These arguments are available in the Lua script through the global `args` table:

```lua
-- Access command line arguments
local minAge = tonumber(args[1]) or 18
local status = args[2] or 'active'

-- Use arguments in your script
local results = sql.query(string.format("SELECT * FROM users WHERE age > %d AND status = '%s'", minAge, status))
for i = 2, #results do
    local row = results[i]
    print(string.format("User %s is %d years old", row[1], row[2]))
end
```

Example usage:
```
.lua-eval "print(args[1])" "hello world"
.lua-eval "local age = tonumber(args[1]); local results = sql.query('SELECT * FROM users WHERE age > ' .. age)" "25"
```

Example with Lua evaluation:

```lua
-- Query and process results
local results = sql.query("SELECT * FROM users WHERE age > 18")
for i = 2, #results do  -- Start from 2 to skip header row
    local row = results[i]
    print(string.format("User %s is %d years old", row[1], row[2]))
end

-- Execute an update
local update = sql.execute("UPDATE users SET status = 'active' WHERE id = 1")
print(string.format("Updated %d rows", update.rows_affected))
```

or use configuration file / environment variables:

## Configuration

tip can be configured in multiple ways:

1. Command-line flags
2. Configuration file (default: `~/.tip/config.toml`)
3. Environment variables
4. `.env` file in the current directory

### Configuration File Format

Create a file named `config.toml` in the `~/.tip/` directory with the following format:

```
host="127.0.0.1"
port="4000"
user="root"
password="your_password"
database="test"
```

### Environment Variables

You can also set the following environment variables:

- `DB_HOST`
- `DB_PORT`
- `DB_USERNAME`
- `DB_PASSWORD`
- `DB_DATABASE`

Once connected, you'll be in an interactive REPL where you can enter SQL queries.

## How to get connection info?

1. Go to [TiDB Cloud](https://tidbcloud.com/), login with your TiDB Cloud account
2. (Optional) Create a new cluster (TiDB Serverless) if you don't have one
3. Click on your cluster
4. Click on `Connect` button on the right top corner
5. Copy the connection info to your config file or environment variables
6. Enjoy 🚀

## Output Formats

tip supports three output formats:

1. Plain: Simple text output
2. Table: Formatted table output (default)
3. JSON: JSON-formatted output

You can specify the output format using the `-o` flag.

## License

Apache 2.0
