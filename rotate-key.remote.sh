#!/usr/bin/env bash
# Runs on the VM. Called by rotate-key.sh as: bash -s -- ACTION ENV_FILE WINDOW_HOURS HEALTH_URL FORCE
# The new key is generated here and never printed or sent anywhere.
set -euo pipefail

action="${1:?acción requerida}"
env_file="${2:?archivo de entorno requerido}"
window_hours="${3:?ventana en horas requerida}"
health_url="${4:?URL de salud requerida}"
force="${5:-}"
marker="$env_file.rotated-at"
current_name="AGENT_TOKEN_ENCRYPTION_KEY"
previous_name="AGENT_TOKEN_ENCRYPTION_KEY_PREVIOUS"

fail() { printf '%s\n' "$*" >&2; exit 1; }

for tool in sudo openssl base64 curl date; do
	command -v "$tool" >/dev/null || fail "Falta la herramienta requerida en la VM: $tool"
done

read_var() {
	sudo cat "$env_file" | awk -F= -v name="$1" '$1 == name { sub(/^[^=]*=/, ""); value = $0 } END { print value }'
}

valid_key() {
	[[ -n "$1" ]] && [[ "$(printf '%s' "$1" | base64 -d 2>/dev/null | wc -c)" -eq 32 ]]
}

write_private() { # write_private DEST < content, atomically and with mode 0600
	local temporary="$1.new"
	sudo install -m 0600 /dev/null "$temporary"
	sudo tee "$temporary" >/dev/null
	sudo mv -f "$temporary" "$1"
}

healthy() {
	curl --fail --silent --show-error "$health_url/healthz" >/dev/null && curl --fail --silent --show-error "$health_url/readyz" >/dev/null
}

restart_and_verify() {
	sudo systemctl restart agentinder && healthy
}

restore_from() { # restore_from BACKUP
	sudo cp -p "$1" "$env_file"
	sudo systemctl restart agentinder || true
}

# Seconds until the window closes; a missing marker counts as just started.
seconds_left() {
	local started now
	now="$(date +%s)"
	started="$(sudo cat "$marker" 2>/dev/null || printf '%s' "$now")"
	printf '%s' $((window_hours * 3600 - (now - started)))
}

without_keys() { # drop both key lines, always ending with a newline
	sudo cat "$env_file" | awk -F= -v a="$current_name" -v b="$previous_name" '$1 != a && $1 != b'
}

format_seconds() {
	local seconds="$1"
	printf '%dh %02dm' $((seconds / 3600)) $(((seconds % 3600) / 60))
}

sudo test -f "$env_file" || fail "No existe $env_file en la VM."
current="$(read_var "$current_name")"
previous="$(read_var "$previous_name")"

case "$action" in
status)
	if [[ -z "$previous" ]]; then
		printf '%s\n' 'Sin rotación en curso.'
	else
		left="$(seconds_left)"
		if ((left > 0)); then
			printf 'Rotación en curso. Faltan %s para poder terminarla.\n' "$(format_seconds "$left")"
		else
			printf '%s\n' 'Rotación en curso. Ya pasó la ventana: puedes ejecutar finish.'
		fi
	fi
	;;
start)
	[[ -z "$previous" ]] || fail "Ya hay una rotación en curso. Termínala con finish antes de iniciar otra."
	valid_key "$current" || fail "$current_name falta o no es una clave base64 de 32 bytes."
	new_key="$(openssl rand -base64 32)"
	valid_key "$new_key" || fail "No se pudo generar una clave válida."
	backup="$env_file.bak-$(date -u +%Y%m%dT%H%M%SZ)"
	sudo cp -p "$env_file" "$backup"
	{
		without_keys
		printf '%s=%s\n' "$current_name" "$new_key"
		printf '%s=%s\n' "$previous_name" "$current"
	} | write_private "$env_file"
	date +%s | write_private "$marker"
	if ! restart_and_verify; then
		restore_from "$backup"
		sudo rm -f "$marker"
		fail "El servicio no pasó healthz/readyz; se restauró la configuración anterior."
	fi
	printf 'Rotación iniciada. La clave anterior queda solo para lectura; termina con finish en %sh.\n' "$window_hours"
	;;
finish)
	[[ -n "$previous" ]] || { printf '%s\n' 'No hay una rotación en curso.'; exit 0; }
	if [[ "$force" != "--force" ]]; then
		left="$(seconds_left)"
		((left <= 0)) || fail "Aún no pasan ${window_hours}h desde el inicio: faltan $(format_seconds "$left"). Usa --force solo si aceptas perder las respuestas protegidas vigentes."
	fi
	backup="$env_file.bak-$(date -u +%Y%m%dT%H%M%SZ)"
	sudo cp -p "$env_file" "$backup"
	sudo cat "$env_file" | awk -F= -v name="$previous_name" '$1 != name' | write_private "$env_file"
	if ! restart_and_verify; then
		restore_from "$backup"
		fail "El servicio no pasó healthz/readyz; se restauró la configuración con la clave anterior."
	fi
	sudo rm -f "$marker"
	sudo sh -c "rm -f '$env_file'.bak-*"
	printf '%s\n' 'Rotación completada. La clave anterior ya no está cargada.'
	;;
*)
	fail "Acción desconocida: $action"
	;;
esac
