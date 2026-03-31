#!/usr/bin/env bash

orchid_select_virt_type() {
  if [[ -e /dev/kvm ]]; then
    printf '%s\n' "${VIRT_TYPE:-kvm}"
  else
    printf '%s\n' "${VIRT_TYPE:-qemu}"
  fi
}

orchid_wait_for_ip() {
  local connect="$1"
  local domain="$2"
  local max_attempts="${3:-60}"
  local sleep_seconds="${4:-2}"
  local ip=""
  local mac=""
  local attempt=""

  for attempt in $(seq 1 "${max_attempts}"); do
    ip="$(virsh -c "${connect}" domifaddr "${domain}" 2>/dev/null | awk '/ipv4/ && ip == "" {split($4,a,"/"); ip=a[1]} END {print ip}')"
    if [[ -z "${ip}" ]]; then
      mac="$(virsh -c "${connect}" domiflist "${domain}" 2>/dev/null | awk 'NR > 2 && $5 != "-" && mac == "" {mac=$5} END {print mac}' | tr '[:upper:]' '[:lower:]')"
      if [[ -n "${mac}" ]]; then
        ip="$(virsh -c "${connect}" net-dhcp-leases default 2>/dev/null | awk -v mac="${mac}" 'tolower($0) ~ mac && /ipv4/ && ip == "" {split($5,a,"/"); ip=a[1]} END {print ip}')"
      fi
    fi
    if [[ -n "${ip}" ]]; then
      printf '%s\n' "${ip}"
      return 0
    fi
    echo "  attempt ${attempt}/${max_attempts}: no IP yet" >&2
    sleep "${sleep_seconds}"
  done

  return 1
}

orchid_wait_for_ssh() {
  local ip="$1"
  local user="${2:-dev}"
  local password="${3:-dev}"
  local max_attempts="${4:-60}"
  local sleep_seconds="${5:-2}"
  local attempt=""

  for attempt in $(seq 1 "${max_attempts}"); do
    if sshpass -p "${password}" ssh \
      -o StrictHostKeyChecking=no \
      -o UserKnownHostsFile=/dev/null \
      -o ConnectTimeout=5 \
      "${user}@${ip}" true >/dev/null 2>&1; then
      return 0
    fi
    echo "  attempt ${attempt}/${max_attempts}: ssh not ready yet" >&2
    sleep "${sleep_seconds}"
  done

  return 1
}

orchid_wait_for_cloud_init() {
  local ip="$1"
  local user="${2:-dev}"
  local password="${3:-dev}"
  if ! sshpass -p "${password}" ssh \
    -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    -o ConnectTimeout=5 \
    "${user}@${ip}" 'sudo cloud-init status --wait && sudo cloud-init status --long'; then
    echo ""
    echo "cloud-init reported a failure. Recent bootstrap log:"
    sshpass -p "${password}" ssh \
      -o StrictHostKeyChecking=no \
      -o UserKnownHostsFile=/dev/null \
      -o ConnectTimeout=5 \
      "${user}@${ip}" 'sudo tail -n 200 /var/log/orchid-bootstrap.log || sudo tail -n 200 /var/log/cloud-init-output.log || true'
    return 1
  fi
}
