# Installing VideoNode

VideoNode is distributed as a Debian package for Linux arm64. This page covers installing it from the APT repository on an RK3588-based board running a Debian-compatible OS with V4L2 capture support.

**Prerequisites:**

- Linux, arm64 (aarch64). VideoNode targets Rockchip RK3588 SBCs for hardware encoding. The recommended board is the [FriendlyElec NanoPC-T6](https://www.friendlyelec.com/index.php?route=product/product&product_id=292) (8GB / 64GB eMMC or higher); other RK3588 boards work but the encoder paths are tuned and validated against the T6.
- A V4L2-capable capture device (e.g. `/dev/video0`): USB or MIPI-CSI cameras, HDMI capture cards. Verify with `v4l2-ctl --list-devices`.
- `curl`, `gpg`, and `apt` available
- A Linux account with `sudo` for the install steps below

## Install required runtime dependencies

### ffmpeg with `h264_rkmpp`

The daemon shells out to `ffmpeg` from `PATH`. On a Rockchip board you need a build with `h264_rkmpp` and `hevc_rkmpp` enabled; stock Debian's `ffmpeg` does not include them. Use the ffmpeg that ships with your board's vendor OS image if it advertises Rockchip support, or install a community build like [nyanmisaka/ffmpeg-rockchip](https://github.com/nyanmisaka/ffmpeg-rockchip). Confirm with:

```bash
ffmpeg -encoders 2>&1 | grep rkmpp
```

After install, run `videonode validate-encoders` to verify the probe finds the hardware encoder. See [Encoders](../operating/encoders) for the precedence rules and example output.

### Rockchip userspace libraries (RGA + MPP)

The native binaries dynamically link `librga` and `librockchip-mpp`. VideoNode's CI builds against pinned releases from [tsukumijima/mpp-rockchip](https://github.com/tsukumijima/mpp-rockchip) and [tsukumijima/librga-rockchip](https://github.com/tsukumijima/librga-rockchip). Install matching `.deb` files from those release pages, or use the equivalents your vendor OS provides. The MPP kernel module must also be loaded (`/proc/mpp_service/load` must exist); vendor images for the NanoPC-T6 load it by default.

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

## Web UI authentication

VideoNode authenticates web UI logins against the Linux user database (`/etc/shadow`). Two things must be true for a user to log in:

1. The user has a regular Linux account on the box.
2. The user is a member of the `videonode` group.

If you ran `apt install` with `sudo`, the postinst script enrolls `$SUDO_USER` into the `videonode` group automatically. Log in to the UI with your usual username and password. Otherwise, add yourself manually:

```bash
sudo adduser "$USER" videonode
```

You'll need to log out and back in (or run `newgrp videonode`) for the new group membership to take effect for new shells. The daemon itself has read access to `/etc/shadow` via its membership in the `shadow` group, granted automatically at install time.

To add another operator later, run the same `adduser ... videonode` command for their account.

### Bypassing Linux auth (basic credentials)

If you'd rather skip the Linux account / group setup entirely (for example on a single-user box, or during initial bring-up), switch the daemon to basic auth. Edit `/etc/videonode/config.toml`:

```toml
[auth]
type     = "basic"
username = "admin"
password = "change-me"
```

Then `sudo systemctl restart videonode`. Log in with the username and password from the file. **Change the defaults before exposing the daemon to a network**: basic auth ships passwords in cleartext over HTTP unless you terminate TLS upstream.

For the full list of `[auth]` and other settings, see [config.toml reference](../reference/config-toml#auth).

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
