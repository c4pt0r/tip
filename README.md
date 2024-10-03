# TiDB CLI

TiDB CLI is a command-line interface tool for interacting with TiDB databases (Serverless). It provides a user-friendly way to connect to TiDB servers, execute queries, and view results in various formats.

## Installation

To install TiDB CLI, make sure you have Go installed on your system, then run:

```
curl -fsSL https://raw.githubusercontent.com/c4pt0r/tidbcli/refs/heads/master/install.sh | sh
```

## Configuration

TiDB CLI can be configured in multiple ways:

1. Command-line flags
2. Configuration file (default: `~/.tidbcli/config`)
3. Environment variables
4. `.env` file in the current directory

### Configuration File Format

Create a file named `config` in the `~/.tidbcli/` directory with the following format:

```
host=127.0.0.1
port=4000
user=root
password=your_password
database=test
```

### Environment Variables

You can also set the following environment variables:

- `DB_HOST`
- `DB_PORT`
- `DB_USERNAME`
- `DB_PASSWORD`
- `DB_DATABASE`

## Usage

Basic usage:

```
tidbcli [flags]
```

Flags:

- `-h`: TiDB Serverless hostname (default: "127.0.0.1")
- `-p`: TiDB port (default: "4000")
- `-u`: TiDB username (default: "root")
- `-P`: TiDB password
- `-d`: TiDB database (default: "test")
- `-c`: Path to configuration file
- `-o`: Output format: plain, table(default), or json

Example:

```
tidbcli -h mytidbserver.com -p 4000 -u myuser -P mypassword -d mydatabase
```

Once connected, you'll be in an interactive REPL where you can enter SQL queries.

## Output Formats

TiDB CLI supports three output formats:

1. Plain: Simple text output
2. Table: Formatted table output (default)
3. JSON: JSON-formatted output

You can specify the output format using the `-o` flag.

## License

Apache 2.0
