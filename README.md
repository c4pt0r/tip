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
