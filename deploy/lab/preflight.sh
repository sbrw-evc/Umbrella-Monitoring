# shellcheck shell=bash

PROBE_IMAGE=${PROBE_IMAGE:-alpine:3.22}

pf_log() { printf '\033[36m==>\033[0m %s\n' "$*"; }
pf_warn() { printf '\033[33mWARN:\033[0m %s\n' "$*" >&2; }
pf_die() { printf '\033[31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

probe() { docker run --rm --pull=missing "$@" "$PROBE_IMAGE" "${PROBE_CMD[@]}" >/dev/null 2>&1; }

apparmor_check() {
  local lab=$1 enabled=N docker_bin so denied
  pf_log "AppArmor"
  [ -r /sys/module/apparmor/parameters/enabled ] && enabled=$(cat /sys/module/apparmor/parameters/enabled)
  if [ "$enabled" = Y ]; then
    pf_log "  AppArmor is enabled in the kernel"
    if ! command -v apparmor_parser >/dev/null 2>&1; then
      if command -v apt-get >/dev/null 2>&1 && [ "$(id -u)" = 0 ]; then
        pf_log "  apparmor_parser is missing: installing the apparmor package (Docker needs it to load docker-default)"
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq apparmor >/dev/null || pf_die "cannot install apparmor"
        systemctl restart docker >/dev/null 2>&1 || true
      else
        pf_die "AppArmor is on but apparmor_parser is missing: install the apparmor package, otherwise Docker cannot load the docker-default profile"
      fi
    fi
    if command -v aa-status >/dev/null 2>&1; then
      if aa-status 2>/dev/null | grep -qw docker-default; then
        pf_log "  profile docker-default is loaded"
      else
        pf_log "  profile docker-default is not loaded yet (Docker loads it when the first container starts)"
      fi
    fi
  else
    pf_log "  AppArmor is not enabled in the kernel: containers run without AppArmor profiles"
  fi

  docker_bin=$(readlink -f "$(command -v docker)" 2>/dev/null || true)
  case "$docker_bin" in
    /snap/*) pf_die "Docker is installed from snap: snap.docker is confined by AppArmor and cannot bind-mount $lab or /var/run/docker.sock for Telegraf and cAdvisor. Remove it (snap remove docker) and install Docker Engine (curl -fsSL https://get.docker.com | sh)" ;;
  esac

  so=$(docker info --format '{{json .SecurityOptions}}' 2>/dev/null || true)
  if [[ "$so" == *apparmor* ]]; then
    pf_log "  Docker uses AppArmor: probing the lab container profiles"
    PROBE_CMD=(true)
    probe --security-opt apparmor=docker-default || pf_die "a container under AppArmor profile docker-default does not start; see: dmesg | grep -i apparmor"
    probe --privileged || pf_die "a privileged container (cAdvisor) does not start under AppArmor"
    PROBE_CMD=(test -S /var/run/docker.sock)
    probe -v /var/run/docker.sock:/var/run/docker.sock:ro || pf_die "AppArmor blocks /var/run/docker.sock in containers (Telegraf reads container logs through it)"
    PROBE_CMD=(test -r /lab/openbao/init.sh)
    probe -v "$lab:/lab:ro" || pf_die "AppArmor blocks the bind mount of $lab in containers"
    pf_log "  docker-default, privileged, docker.sock and bind mounts work"
  else
    pf_log "  Docker does not use AppArmor"
  fi

  if [ "$enabled" = Y ] && command -v journalctl >/dev/null 2>&1; then
    denied=$(journalctl -k --since "-15min" --no-pager 2>/dev/null | grep 'apparmor="DENIED"' | grep -Ei 'docker|containerd|runc' | tail -5)
    [ -z "$denied" ] || pf_warn "recent AppArmor denials for containers:"$'\n'"$denied"
  fi
}
