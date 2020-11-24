# sshmon-check-postgres
Nagios/Checkmk-compatible SSHMon-check for Postgres-Databases

## Installation
* Download [latest Release](https://github.com/indece-official/sshmon-check-postgres/releases/latest)
* Move binary to `/usr/local/bin/sshmon_check_postgres`


## Usage
```
$> sshmon_check_postgres -service Postgres_testdbserver.default -dns 10.96.0.10:53 -host testdbserver.default.svc.cluster.local -db testdb -user testuser -password testpassword
```

```
Usage of sshmon_check_postgres:
  -conntimeout int
        Connection timeout (default 5)
  -db string
        Database
  -dns string
        Use alternate dns server
  -host string
        Host
  -maxlockage int
        Maximum lock age in seconds
  -maxqueryduration int
        Maximum query duration in seconds
  -password string
        Password
  -passwordfile string
        File to read password from
  -port int
        Port (default 5432)
  -service string
        Service name (defaults to Elasticsearch_<host>)
  -user string
        User
  -v    Print the version info and exit
```

Output:
```
0 Postgres_testdbserver.default - OK - Postgres database 'testdb' on testdbserver.default.svc.cluster.local is up and running
```

### Supported Postgres versions
| Version | Tested |
| --- | --- |
| v7 | Yes |

## Development
### Snapshot build

```
$> make --always-make
```

### Release build

```
$> BUILD_VERSION=1.0.0 make --always-make
```