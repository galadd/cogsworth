# Cogsworth
A lightweight container management tool that maintains desired container state using BoltDB and Docker. It runs a reconciliation loop that continuously ensures containers match their desired state. It stores container specifications in BoltDB and uses Docker to manage the actual container lifecycle.

## Installation

### Build from source
```bash
cd cogsworth
go build -o cogs .
```

### Run
```bash
# to start the program 
./cogs start
```

```bash
# to add a container
./cogs add <image> <host_port>:<container_port>
```

```bash
# to delete a container
./cogs rm <container_id>
```

```bash
# list all containers
./cogs ls
```

```bash
# clean up all
./cogs clean
```
