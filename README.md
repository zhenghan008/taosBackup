

# TDengine 数据库备份恢复工具
[英文版本](./README_EN.md) | 中文

一款功能强大的 TDengine（涛思数据库）备份和恢复工具，支持全量备份、增量备份、数据恢复等功能。

## 功能特性

- ✅ **全量备份** – 一次性备份整个数据库
- ✅ **增量备份** – 支持从前一天零点到当前时间的增量备份
- ✅ **CSV 导出** – 自动将数据导出为 CSV 格式，边导出边压缩，减少磁盘占用空间，支持分片处理大数据集
- ✅ **数据恢复** – 从备份文件目录恢复数据到 TDengine 数据库
- ✅ **并发控制** – 可配置工作线程数，充分利用服务器资源
- ✅ **灵活配置** – 支持命令行参数配置所有备份参数

## 安装

### 前置条件

- Go 1.25 或更高版本
- TDengine 数据库已安装并正常运行
- 备份目录有写入权限

### 编译

```bash
git clone <repository-url>
cd taosBackup
go build -o taosBackup
```
## 使用说明

### 基本命令

```bash
./taosBackup [选项]
```

### 命令行参数

| 参数   | 长参数 | 默认值                 | 说明 |
|------|--------|---------------------|------|
| `-h` | - | `localhost:6041`    | TDengine 服务器地址，格式：`host:port` |
| `-u` | - | `root`              | TDengine 用户名 |
| `-p` | - | `taosdata`          | TDengine 密码 |
| `-d` | - | 空                   | 目标数据库名称（必填） |
| `-w` | - | CPU 核心数/3           | 工作线程数限制，建议不超过 CPU 核心数 |
| `-f` | - | `false`             | 是否执行全量备份；`false` 时为增量备份（从前一天零点到当前时间） |
| `-r` | - | `100000`            | 每个 CSV 文件的最大行数 |
| `-b` | - | `/data/taos_Backup` | 备份文件存储路径 |
| `-m` | - | `e`                 | 运行模式：`e` 为备份模式，`i` 为恢复模式 |
| `-s` | - | 空                   |指定超级表的名称，多个表名之间用逗号分隔，例如 stableNameA,stableNameB,... 或 stableNameA |
| `-o` | - | 空                   |指定普通表的名称，多个表名用逗号分隔，例如 otableNameA,otableNameB,... 或 otableNameA |
---

## 使用示例

### 1. 全量备份

```bash
./taosBackup -h 192.168.1.100:6041 -u root -p taosdata -d mydb -f true -b /data/taos_Backup/mydb
```

**参数说明：**
- 连接到 `192.168.1.100:6041` 的 TDengine 服务器
- 使用用户名 `root`，密码 `taosdata`
- 备份数据库 `mydb` 的所有数据
- 备份文件保存到 `/data/taos_Backup/mydb` 目录

### 2. 增量备份

```bash
./taosBackup -h localhost:6041 -u root -p taosdata -d mydb -f false -b /data/taos_Backup/mydb -w 4
```

**参数说明：**
- 备份从前一天零点到现在的新增数据
- 使用 4 个工作线程并发处理
- 备份文件保存到 `/data/taos_Backup/mydb`

### 3. CSV 导出（分片处理）

```bash
./taosBackup -h localhost:6041 -d mydb -m e -r 50000 -b /backup/csv
```

**参数说明：**
- 导出数据库 `mydb` 的数据为 CSV 格式
- 每个 CSV 文件最多包含 50000 行
- 自动分片处理大数据集

### 4. 恢复数据

```bash
./taosBackup -h localhost:6041 -u root -p taosdata -d mydb -m i -b /data/taos_Backup
```

**参数说明：**
- 运行在恢复模式（`-m i`）
- 恢复到数据库 `mydb`
- 从 `/data/taos_Backup` 读取备份文件（**恢复前必须先在 TDengine 中创建此超级表或者普通表的表结构**）

---

## 重要注意事项

### ⚠️ 恢复模式前置条件

在使用恢复模式（`-m i`）之前，**必须**在 TDengine 数据库中提前创建好超级表或者普通表的表结构：

```sql
-- 连接到 TDengine 后执行
USE mydb;
CREATE TABLE mysupertable (
    ts TIMESTAMP,
    value DOUBLE,
    status INT
) TAGS (location BINARY(64), device_id INT);
```

### 💾 备份路径权限
如果目录不存在，程序会自动创建指定的备份目录

确保备份目录存在且用户有写入权限：

```bash
chmod 755 /data/TDengineBackup
```

### 🔧 工作线程建议

- 默认使用 CPU 核心数的三分之一
- 可适当增加线程数
- 不建议超过 CPU 核心总数（会导致上下文切换开销增加）

### 📊 CSV 行数设置

- 数据量大时，建议将 `-r` 设置为 10000 ~ 500000
- 过大会导致单个文件过大，难以处理
- 过小会产生过多小文件，增加存储和处理开销

---


## 许可证

本项目采用[GNU较宽公共许可证v3.0]（./LICENSE）授权。详情请参见[LICENSE](./LICENSE)文件。

## 贡献

欢迎提交 Issue 和 Pull Request！

## 联系方式

如有问题，请通过以下方式联系：
- GitHub Issues: https://github.com/zhenghan008/taosBackup/issues
```