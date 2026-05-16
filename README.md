[中文版本](./README_CN) | English
# TDengine Database Backup and Restore Tool

A powerful backup and restore tool for TDengine (Taos Time-Series Database), supporting full backup, incremental backup, and data recovery.

## Features

- ✅ **Full Backup** – Complete backup of the entire database at once
- ✅ **Incremental Backup** – Support incremental backup from previous day's midnight to current time
- ✅ **CSV Export** – Automatically export data to CSV format with on-the-fly compression to reduce disk space usage and support chunked processing for  datasets
- ✅ **Data Recovery** – Restore data to TDengine from backup directories
- ✅ **Concurrency Control** – Configurable worker threads to fully utilize server resources
- ✅ **Flexible Configuration** – Support command-line parameters to configure all backup options

## Installation

### Prerequisites

- Go 1.25 or higher
- TDengine database installed and running
- Write permission to the backup directory

### Build

```bash
git clone https://github.com/zhenghan008/taosBackup.git
cd taosBackup
go build -o taosBackup
```

## Usage

### Basic Command

```bash
./taosBackup [options]
```

### Command-line Parameters

| Parameter | Long Form | Default             | Description |
|-----------|-----------|---------------------|-------------|
| `-h`      | - | `localhost:6041`    | TDengine server address in format `host:port` |
| `-u`      | - | `root`              | TDengine username |
| `-p`      | - | `taosdata`          | TDengine password |
| `-d`      | - | empty               | Target database name (required) |
| `-w`      | - | CPU cores/3         | Worker thread limit; recommended not to exceed CPU core count |
| `-f`      | - | `false`             | Whether to perform full backup; `false` performs incremental backup (from previous day midnight to now) |
| `-r`      | - | `100000`            | Maximum rows per CSV file |
| `-b`      | - | `/data/taos_Backup` | Backup file storage path |
| `-m`      | - | `e`                 | Running mode: `e` for backup mode, `i` for restore mode |
| `-s`      | - | empty               | Specify supertable name(s), separated by commas, e.g., stableNameA,stableNameB,... or stableNameA |
| `-o`      | - | empty               | Specify regular table name(s), separated by commas, e.g., otableNameA,otableNameB,... or otableNameA |
| `-P`      | - | `M`                 |Specify the time precision for the database, where M represents milliseconds, m represents microseconds, and n represents nanoseconds. This must be specified according to the actual database; otherwise, data cannot be exported. The default is milliseconds. |
| `-i`      | - | 1                   |Setting the interval in days determines the step size for exporting data. For a full backup, this interval indicates how many days will be used to export the data in stages. For an incremental backup, it means only the data within the specified interval will be exported. |
---

## Usage Examples

### 1. Full Backup

```bash
./taosBackup -h 192.168.1.100:6041 -u root -p taosdata -d mydb -f true -b /data/taos_Backup/mydb
```

**Parameter Description:**
- Connect to TDengine server at `192.168.1.100:6041`
- Use username `root` and password `taosdata`
- Backup all data from database `mydb`
- Save backup files to `/data/taos_Backup/mydb` directory

### 2. Incremental Backup

```bash
./taosBackup -h localhost:6041 -u root -p taosdata -d mydb -f false -b /data/taos_Backup/mydb -w 4
```

**Parameter Description:**
- Backup new data from previous day's midnight to now
- Use 4 worker threads for concurrent processing
- Save backup files to `/data/taos_Backup/mydb`

### 3. CSV Export (Chunked Processing)

```bash
./taosBackup -h localhost:6041 -d mydb -m e -r 50000 -b /backup/csv
```

**Parameter Description:**
- Export data from database `mydb` to CSV format
- Maximum 50000 rows per CSV file
- Automatically chunk large datasets

### 4. Data Recovery

```bash
./taosBackup -h localhost:6041 -u root -p taosdata -d mydb -m i -b /data/taos_Backup
```

**Parameter Description:**
- Run in restore mode (`-m i`)
- Restore to database `mydb`
- Read backup files from `/data/taos_Backup` (**Table structure must be created in TDengine before recovery**)

---

## Important Notes

### ⚠️ Prerequisites for Restore Mode

Before using restore mode (`-m i`), **you must** pre-create the table structure in TDengine:

```sql
-- Execute after connecting to TDengine
USE mydb;
CREATE TABLE mysupertable (
    ts TIMESTAMP,
    value DOUBLE,
    status INT
) TAGS (location BINARY(64), device_id INT);
```

### 💾 Backup Directory Permissions

Ensure the backup directory exists and the user has write permission:

```bash
chmod 755 /data/TDengineBackup
```

If the directory does not exist, the program will automatically create it.

### 🔧 Worker Thread Recommendations

- Default uses one-third of CPU core count
- Can appropriately increase thread count
- Not recommended to exceed total CPU cores (causes context-switching overhead)

### 📊 CSV Row Count Settings

- For large datasets, recommend setting `-r` between 10000 ~ 500000
- Too large causes single files to become unwieldy
- Too small generates many small files, increasing storage and processing overhead

---

## License

This project is licensed under the [GNU Lesser General Public License v3.0](./LICENSE). See the [LICENSE](./LICENSE) file for details.

## Contributing

Issues and Pull Requests are welcome!

## Contact

If you have any questions, please contact us through:
- GitHub Issues: https://github.com/zhenghan008/taosBackup/issues
```