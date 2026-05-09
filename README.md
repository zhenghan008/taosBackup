# TaOS 数据库备份恢复工具

一款功能强大的 TaOS（涛思数据库）备份和恢复工具，支持全量备份、增量备份、CSV 导出和数据恢复等功能。

## 功能特性

- ✅ **全量备份** – 一次性备份整个数据库
- ✅ **增量备份** – 支持从前一天零点到当前时间的增量备份
- ✅ **CSV 导出** – 自动将数据导出为 CSV 格式，支持分片处理大数据集
- ✅ **数据恢复** – 从备份文件恢复数据到 TaOS 数据库
- ✅ **并发控制** – 可配置工作线程数，充分利用服务器资源
- ✅ **灵活配置** – 支持命令行参数配置所有备份参数

## 安装

### 前置条件

- Go 1.13 或更高版本
- TaOS 数据库已安装并正常运行
- 备份目录有写入权限

### 编译

```bash
git clone <repository-url>
cd taos-backup
go build -o taos-backup
用户要求从"使用说明"开始的所有内容放到md文档里，并且是直接可以复制的md文档。我需要提供一个完整的markdown代码块，包含从"## 使用说明"开始的所有内容。```markdown
## 使用说明

### 基本命令

```bash
./taos-backup [选项]
```

### 命令行参数

| 参数 | 长参数 | 默认值 | 说明 |
|------|--------|--------|------|
| `-h` | - | `localhost:6041` | TaOS 服务器地址，格式：`host:port` |
| `-u` | - | `root` | TaOS 用户名 |
| `-p` | - | `taosdata` | TaOS 密码 |
| `-d` | - | 空 | 目标数据库名称（必填） |
| `-w` | - | CPU 核心数/3 | 工作线程数限制，建议不超过 CPU 核心数 |
| `-f` | - | `false` | 是否执行全量备份；`false` 时为增量备份（从前一天零点到当前时间） |
| `-r` | - | `100000` | 每个 CSV 文件的最大行数 |
| `-b` | - | `/data/taosBackup` | 备份文件存储路径 |
| `-m` | - | `e` | 运行模式：`e` 为备份模式，`i` 为恢复模式 |
| `-s` | - | 空 | 指定超级表名称（恢复模式必填） |

---

## 使用示例

### 1. 全量备份

```bash
./taos-backup -h 192.168.1.100:6041 -u root -p taosdata -d mydb -f true -b /backup/taos
```

**参数说明：**
- 连接到 `192.168.1.100:6041` 的 TaOS 服务器
- 使用用户名 `root`，密码 `taosdata`
- 备份数据库 `mydb` 的所有数据
- 备份文件保存到 `/backup/taos` 目录

### 2. 增量备份

```bash
./taos-backup -h localhost:6041 -u root -p taosdata -d mydb -f false -b /data/taosBackup -w 4
```

**参数说明：**
- 备份从前一天零点到现在的新增数据
- 使用 4 个工作线程并发处理
- 备份文件保存到 `/data/taosBackup`

### 3. CSV 导出（分片处理）

```bash
./taos-backup -h localhost:6041 -d mydb -m e -r 50000 -b /backup/csv
```

**参数说明：**
- 导出数据库 `mydb` 的数据为 CSV 格式
- 每个 CSV 文件最多包含 50000 行
- 自动分片处理大数据集

### 4. 恢复数据

```bash
./taos-backup -h localhost:6041 -u root -p taosdata -d mydb -m i -s mysupertable -b /backup/taos
```

**参数说明：**
- 运行在恢复模式（`-m i`）
- 恢复到数据库 `mydb`
- 指定超级表 `mysupertable`（**恢复前必须先在 TaOS 中创建此超级表结构**）
- 从 `/backup/taos` 读取备份文件

---

## 重要注意事项

### ⚠️ 恢复模式前置条件

在使用恢复模式（`-m i`）之前，**必须**在 TaOS 数据库中提前创建好超级表（Super Table）结构：

```sql
-- 连接到 TaOS 后执行
USE mydb;
CREATE TABLE mysupertable (
    ts TIMESTAMP,
    value DOUBLE,
    status INT
) TAGS (location BINARY(64), device_id INT);
```

### 💾 备份路径权限

确保备份目录存在且用户有写入权限：

```bash
mkdir -p /data/taosBackup
chmod 755 /data/taosBackup
```

### 🔧 工作线程建议

- 默认使用 CPU 核心数的三分之一
- 对于 I/O 密集型操作，可适当增加线程数
- 不建议超过 CPU 核心总数（会导致上下文切换开销增加）

### 📊 CSV 行数设置

- 数据量大时，建议将 `-r` 设置为 10000 ~ 100000
- 过大会导致单个文件过大，难以处理
- 过小会产生过多小文件，增加存储和处理开销

---

## 故障排查

### 连接失败

```
Error: connect to TaOS failed
```

**解决方案：**
- 检查 TaOS 服务是否正常运行：`taos -h localhost`
- 验证地址和端口是否正确
- 检查防火墙规则

### 数据库不存在

```
Error: database not found
```

**解决方案：**
- 确保使用 `-d` 指定了正确的数据库名称
- 在 TaOS 中预先创建数据库：`CREATE DATABASE mydb;`

### 恢复时超级表不存在

```
Error: super table not found
```

**解决方案：**
- 在恢复前，使用 `-s` 指定的超级表名称必须在 TaOS 中提前创建
- 超级表的字段结构必须与备份数据匹配

### 备份路径权限不足

```
Error: permission denied
```

**解决方案：**
- 检查并提升备份目录权限：`sudo chmod -R 755 /data/taosBackup`
- 或以管理员身份运行工具

---

## 性能优化建议

| 场景 | 建议配置 |
|------|---------|
| 小型数据库（<1GB） | `-w 2 -r 100000` |
| 中型数据库（1GB~100GB） | `-w 4 -r 50000` |
| 大型数据库（>100GB） | `-w 8 -r 10000` |
| 高速 SSD 存储 | 增加 `-w` 值至 CPU 核心数 |
| 网络环境较差 | 减少 `-w` 值，增加超时配置 |

---

## 常见问题 (FAQ)

**Q: 增量备份的时间范围是多少？**  
A: 从前一天的 00:00:00 到当前备份执行时刻。

**Q: 备份过程中能否停止工具？**  
A: 可以按 `Ctrl+C` 停止，但可能导致备份不完整。建议在备份完成后再停止。

**Q: 恢复失败后能否重新恢复？**  
A: 可以，重新运行恢复命令即可（如果超级表存在，需要清空或删除后重建）。

**Q: 支持哪些操作系统？**  
A: Linux、macOS、Windows（需要 Go 环境）。

---

## 许可证

[选择适合的许可证，如 MIT、Apache 2.0 等]

## 贡献

欢迎提交 Issue 和 Pull Request！

## 联系方式

如有问题，请通过以下方式联系：
- GitHub Issues: <issues-url>
- 邮箱: <your-email>
```

复制上面的内容到你的 README.md 文件即可！