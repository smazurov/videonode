# Installing VideoNode

VideoNode is distributed as a Debian package for Linux arm64. This page covers installing it from the APT repository on an RK3588-based board running a Debian-compatible OS with V4L2 capture support.

**Prerequisites:**

- Linux, arm64 (aarch64)
- A V4L2-capable capture device (e.g. `/dev/video0`)
- `curl`, `gpg`, and `apt` available

## Install from the APT repository

Follow these steps in order.

1. To trust the repository's signing key, download and install it:

    ```bash
    curl -fsSL https://mazurov.dev/videonode/public.key \
      | gpg --dearmor \
      | sudo tee /usr/share/keyrings/videonode-archive-keyring.gpg > /dev/null
    ```

2. To add the repository to your APT sources, write the sources entry:

    ```bash
    echo "deb [arch=arm64 signed-by=/usr/share/keyrings/videonode-archive-keyring.gpg] \
      https://mazurov.dev/videonode stable main" \
      | sudo tee /etc/apt/sources.list.d/videonode.list
    ```

3. To fetch the updated package index, run:

    ```bash
    sudo apt update
    ```

4. To install VideoNode, run:

    ```bash
    sudo apt install videonode
    ```

The post-install script creates a `videonode` system user, writes default configuration to `/etc/videonode/config.toml`, enables `videonode.service`, and starts it immediately.

## Verify the service is running

To confirm the service started, check its status:

```bash
systemctl status videonode.service
```

To follow live logs:

```bash
journalctl -u videonode.service -f
```

The API is available at `http://localhost:8090` once the service is up. See the [quickstart](quickstart) for first steps after installation.
