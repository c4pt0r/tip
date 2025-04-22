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
- `.lua-eval-file <filename|url> [args...]` - Execute a Lua script from a file or URL with SQL integration

### Lua Integration

The `.lua-eval` command allows you to execute Lua scripts with direct SQL integration. It provides two main functions:

- `sql.query(query)` - Execute a SELECT query and return a Result object
- `sql.execute(query)` - Execute an INSERT/UPDATE/DELETE query and return a Result object

Both functions return a Result object with the following structure:
- `ok`: boolean indicating success (true) or failure (false)
- `error`: error message if any (empty string if successful)
- For query results:
  - `data`: table containing the query results (rows)
  - `columns`: table containing column names
  - `row_count`: number of rows returned
- For execute results:
  - `rows_affected`: number of rows affected
  - `last_insert_id`: ID of the last inserted row

You can pass arguments to your Lua script after the script string. These arguments are available in the Lua script through the global `args` table:

```lua
-- Access command line arguments
local minAge = tonumber(args[1]) or 18
local status = args[2] or 'active'

-- Use arguments in your script
local result = sql.query(string.format("SELECT * FROM users WHERE age > %d AND status = '%s'", minAge, status))
if result.ok then
    -- Process the data
    for i, row in ipairs(result.data) do
        print(string.format("User %s is %d years old", row[1], row[2]))
    end
else
    -- Handle error
    print("Error: " .. result.error)
end
```

Example usage:
```
.lua-eval "print(args[1])" "hello world"
.lua-eval "local age = tonumber(args[1]); local result = sql.query('SELECT * FROM users WHERE age > ' .. age); if result.ok then for i, row in ipairs(result.data) do print(row[1]) end end" "25"
```

Example with Lua evaluation:

```lua
-- Query and process results
local result = sql.query("SELECT * FROM users WHERE age > 18")
if result.ok then
    -- Process the data
    for i, row in ipairs(result.data) do
        print(string.format("User %s is %d years old", row[1], row[2]))
    end
    print(string.format("Total rows: %d", result.row_count))
else
    -- Handle error
    print("Error: " .. result.error)
end

-- Execute an update
local result = sql.execute("UPDATE users SET status = 'active' WHERE id = 1")
if result.ok then
    print(string.format("Updated %d rows", result.rows_affected))
else
    print("Error: " .. result.error)
end
```

### HTTP Integration

The Lua integration also provides HTTP functionality through the `http.fetch` function, which allows you to make HTTP requests from your Lua scripts:

```lua
-- Basic GET request
local success, response = http.fetch("GET", "https://api.example.com/data", nil, "", nil)
if success then
    print("Status code:", response.status_code)
    print("Response body:", response.body)
    -- Access headers
    for k, v in pairs(response.headers) do
        print(k .. ": " .. v)
    end
else
    print("Error:", response)
end

-- POST request with headers and body
local headers = {
    ["Content-Type"] = "application/json",
    ["Authorization"] = "Bearer token123"
}
local body = '{"name": "John", "age": 30}'
local success, response = http.fetch("POST", "https://api.example.com/users", headers, body, nil)
if success then
    print("Status code:", response.status_code)
    print("Response body:", response.body)
else
    print("Error:", response)
end

-- Asynchronous request with callback
local function handleResponse(success, response)
    if success then
        print("Async request completed with status:", response.status_code)
        print("Response body:", response.body)
    else
        print("Async request failed:", response)
    end
end

http.fetch("GET", "https://api.example.com/data", nil, "", handleResponse)
print("Request sent asynchronously, continuing execution...")
```

The `http.fetch` function accepts the following parameters:
- `method`: HTTP method (GET, POST, PUT, DELETE, etc.)
- `url`: The URL to request
- `headers`: A table of HTTP headers (optional)
- `body`: Request body (optional)
- `callback`: A function to handle the response asynchronously (optional)

The response object contains:
- `status_code`: HTTP status code
- `body`: Response body as a string
- `headers`: A table of response headers

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
