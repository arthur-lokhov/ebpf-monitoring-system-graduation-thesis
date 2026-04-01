# eBPF Monitoring - Docker Deployment

Docker Compose deployment configuration for eBPF Monitoring platform.

## Quick Start

### 1. First Time Setup

```bash
cd deployments

# Run initial setup (creates .env and directories)
./deploy.sh setup

# Edit configuration
nano .env

# Start services
./deploy.sh start
```

### 2. Using Environment Variables

All configuration is managed through the `.env` file:

```bash
# Copy template
cp .env.example .env

# Edit with your values
nano .env
```

### 3. Start Services

```bash
# Start all services
./deploy.sh start

# Or with docker-compose directly
docker-compose up -d
```

## Configuration Options

### PostgreSQL

| Variable | Default | Description |
|----------|---------|-------------|
| `POSTGRES_USER` | `epbf` | Database username |
| `POSTGRES_PASSWORD` | `epbf_password` | Database password |
| `POSTGRES_DB` | `epbf` | Database name |
| `POSTGRES_PORT` | `5432` | Database port |
| `POSTGRES_DATA_PATH` | `./postgres/data` | Data directory |

### Garage S3

| Variable | Default | Description |
|----------|---------|-------------|
| `GARAGE_ADMIN_TOKEN` | `GKfa...` | S3 access key |
| `GARAGE_SECRET_KEY` | `9215...` | S3 secret key |
| `GARAGE_REGION` | `garage` | Region name |
| `GARAGE_BUCKET` | `epbf-plugins` | Default bucket |
| `GARAGE_PORT` | `3900` | S3 API port |

### epbf-monitor

| Variable | Default | Description |
|----------|---------|-------------|
| `MONITOR_PORT` | `8080` | HTTP API port |
| `MONITOR_LOG_LEVEL` | `info` | Log level |
| `ENABLE_DOCKER` | `true` | Enable Docker builder |
| `BUILD_DIR` | `/tmp/epbf-builds` | Plugin build directory |
| `BUILDER_IMAGE` | `epbf-monitor-builder:latest` | Builder image |

### Security

| Variable | Default | Description |
|----------|---------|-------------|
| `PRIVILEGED_MODE` | `true` | Run in privileged mode |
| `CAP_ADD` | `SYS_ADMIN,BPF,PERFMON,IPC_LOCK` | Capabilities to add |
| `MEMLOCK_LIMIT` | `-1` | Memory lock limit (bytes) |

## Deployment Script Commands

```bash
# First-time setup
./deploy.sh setup

# Start services
./deploy.sh start

# Stop services
./deploy.sh stop

# Restart services
./deploy.sh restart

# View logs
./deploy.sh logs

# Check status
./deploy.sh status

# Clean everything (removes data!)
./deploy.sh clean

# Rebuild epbf-monitor
./deploy.sh rebuild
```

## Manual Docker Compose Commands

```bash
# Start services
docker-compose up -d

# Stop services
docker-compose down

# View logs
docker-compose logs -f

# Rebuild service
docker-compose build --no-cache epbf-monitor
docker-compose up -d epbf-monitor

# Scale services (if supported)
docker-compose up -d --scale epbf-monitor=2
```

## Access Points

After deployment:

| Service | URL | Description |
|---------|-----|-------------|
| epbf-monitor | http://localhost:8080 | Main API server |
| React UI | http://localhost:3000 | Web interface (dev) |
| PostgreSQL | localhost:5432 | Database |
| Garage S3 | localhost:3900 | Object storage |

## Environment-Specific Configurations

### Development

```bash
# .env for local development
MONITOR_LOG_LEVEL=debug
ENABLE_DOCKER=true
PRIVILEGED_MODE=true
```

### Production

```bash
# .env for production
POSTGRES_PASSWORD=<strong-password>
GARAGE_SECRET_KEY=<strong-secret>
MONITOR_LOG_LEVEL=info
PRIVILEGED_MODE=false
CAP_ADD=BPF,PERFMON
MEMLOCK_LIMIT=67108864
```

### Linux Server (with eBPF support)

```bash
# .env for Linux server
POSTGRES_HOST=localhost
GARAGE_HOST=localhost
ENABLE_DOCKER=true
PRIVILEGED_MODE=true
```

## Troubleshooting

### Services won't start

```bash
# Check logs
./deploy.sh logs

# Check if ports are in use
lsof -i :8080
lsof -i :5432
lsof -i :3900
```

### eBPF programs fail to load

1. Ensure kernel supports eBPF (4.9+)
2. Run with privileged mode or add capabilities
3. Check memlock limits

```bash
# Check eBPF support
uname -r
bpftool version

# Increase memlock limit
ulimit -l 67108864
```

### Database connection fails

```bash
# Check PostgreSQL is running
docker-compose ps postgres

# Test connection
docker-compose exec postgres pg_isready -U epbf

# View PostgreSQL logs
docker-compose logs postgres
```

### S3 upload fails

```bash
# Check Garage is running
docker-compose ps garage

# Test S3 connection
curl http://localhost:3900

# View Garage logs
docker-compose logs garage
```

## Backup and Restore

### Backup Database

```bash
docker-compose exec postgres pg_dump -U epbf epbf > backup.sql
```

### Restore Database

```bash
docker-compose exec -T postgres psql -U epbf epbf < backup.sql
```

### Backup Garage Data

```bash
tar -czf garage-backup.tar.gz ./garage/data
```

## Updating

```bash
# Pull latest images
docker-compose pull

# Rebuild and restart
./deploy.sh rebuild

# Or full restart
docker-compose down
docker-compose up -d
```

## Resource Requirements

| Component | CPU | Memory | Disk |
|-----------|-----|--------|------|
| PostgreSQL | 1 core | 512MB | 10GB |
| Garage S3 | 1 core | 256MB | 100GB+ |
| epbf-monitor | 2 cores | 512MB | 1GB |
| **Total** | **4 cores** | **1.3GB** | **111GB+** |

## Security Considerations

1. **Change default passwords** in `.env`
2. **Use privileged mode only when necessary**
3. **Limit capabilities** in production
4. **Enable TLS** for external access
5. **Use firewall** to restrict access
6. **Regular backups** of database and S3 data

## Network Configuration

The deployment uses:
- Bridge network for epbf-monitor and postgres
- Host network for Garage (required for performance)

To use custom network:

```yaml
networks:
  epbf-network:
    driver: bridge
    ipam:
      config:
        - subnet: 172.28.0.0/16
```

## License

Apache 2.0
