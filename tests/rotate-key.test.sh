#!/usr/bin/env bash
# Prueba rotate-key.remote.sh sin VM: sudo, systemctl y curl se sustituyen por dobles.
set -uo pipefail
cd -- "$(dirname -- "$0")/.."
script="$PWD/rotate-key.remote.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
stubs="$work/stubs"
mkdir -p "$stubs"
printf '#!/usr/bin/env bash\nexec "$@"\n' >"$stubs/sudo"
printf '#!/usr/bin/env bash\necho "$@" >>"$STUB_LOG"\n' >"$stubs/systemctl"
printf '#!/usr/bin/env bash\n[[ -f "$STUB_DIR/health-fails" ]] && exit 22\nexit 0\n' >"$stubs/curl"
chmod +x "$stubs"/*

passed=0
failed=0
check() { # check DESCRIPTION CONDITION...
	local description="$1"
	shift
	if "$@"; then
		passed=$((passed + 1))
	else
		failed=$((failed + 1))
		printf 'FALLA: %s\n' "$description"
	fi
}

key_a="$(printf 'A%.0s' {1..32} | base64)"
env_file=""
output=""
status=0

fresh() { # new sandbox with an env file that has no trailing newline
	rm -rf "$work/vm"
	mkdir -p "$work/vm"
	env_file="$work/vm/agentinder.env"
	printf 'APP_ENV=production\nDATA_PATH=/x\nAGENT_TOKEN_ENCRYPTION_KEY=%s' "$key_a" >"$env_file"
	chmod 0600 "$env_file"
	export STUB_LOG="$work/vm/systemctl.log" STUB_DIR="$work/vm"
	: >"$STUB_LOG"
}
run() { # run ACTION [force]
	output="$(PATH="$stubs:$PATH" bash "$script" "$1" "$env_file" 24 http://127.0.0.1:8080 "${2:-}" 2>&1)"
	status=$?
}
value() { awk -F= -v n="$1" '$1 == n { sub(/^[^=]*=/, ""); v = $0 } END { print v }' "$env_file"; }
restarts() { wc -l <"$STUB_LOG" | tr -d ' '; }
backups() { ls "$env_file".bak-* 2>/dev/null | wc -l | tr -d ' '; }
age_marker() { printf '%s\n' "$(($(date +%s) - $1))" >"$env_file.rotated-at"; }
is() { [[ "$1" == "$2" ]]; }
differs() { [[ "$1" != "$2" ]]; }
has() { [[ "$1" == *"$2"* ]]; }
lacks() { [[ "$1" != *"$2"* ]]; }

fresh; run status
check "status sin rotación" has "$output" "Sin rotación en curso"

fresh; run start
new_key="$(value AGENT_TOKEN_ENCRYPTION_KEY)"
check "start termina bien" is "$status" 0
check "la clave actual cambia" differs "$new_key" "$key_a"
check "la clave nueva mide 32 bytes" is "$(printf '%s' "$new_key" | base64 -d | wc -c | tr -d ' ')" 32
check "la anterior queda como PREVIOUS" is "$(value AGENT_TOKEN_ENCRYPTION_KEY_PREVIOUS)" "$key_a"
check "las demás variables se conservan" is "$(value APP_ENV)$(value DATA_PATH)" "production/x"
check "cada variable en su línea (el archivo no acababa en salto)" is "$(grep -c '' "$env_file")" 4
check "el archivo sigue en modo 0600" is "$(stat -c %a "$env_file")" 600
check "reinicia el servicio una vez" is "$(restarts)" 1
check "hay una copia de seguridad" is "$(backups)" 1
check "la salida no muestra ninguna clave" lacks "$output" "$new_key"
check "tampoco la anterior" lacks "$output" "$key_a"

run status
check "status avisa de la rotación" has "$output" "Rotación en curso. Faltan"

before="$(cat "$env_file")"
run start
check "no inicia otra rotación encima" is "$status" 1
check "y deja el archivo intacto" is "$(cat "$env_file")" "$before"

run finish
check "finish antes de la ventana se rechaza" is "$status" 1
check "explica cuánto falta" has "$output" "Aún no pasan 24h"
check "sigue la clave anterior cargada" is "$(value AGENT_TOKEN_ENCRYPTION_KEY_PREVIOUS)" "$key_a"

age_marker $((24 * 3600 + 5))
run finish
check "finish tras la ventana termina bien" is "$status" 0
check "se retira la clave anterior" is "$(value AGENT_TOKEN_ENCRYPTION_KEY_PREVIOUS)" ""
check "la clave nueva se conserva" is "$(value AGENT_TOKEN_ENCRYPTION_KEY)" "$new_key"
check "se borra la marca" test ! -e "$env_file.rotated-at"
check "se borran las copias con claves viejas" is "$(backups)" 0
run status
check "status ya no ve rotación" has "$output" "Sin rotación en curso"

fresh; run start; run finish --force
check "finish --force salta la espera" is "$status" 0
check "y retira la anterior" is "$(value AGENT_TOKEN_ENCRYPTION_KEY_PREVIOUS)" ""

fresh; touch "$work/vm/health-fails"; before="$(cat "$env_file")"; run start
check "start con servicio enfermo falla" is "$status" 1
check "restaura el archivo original" is "$(cat "$env_file")" "$before"
check "sin marca de rotación" test ! -e "$env_file.rotated-at"
check "reinició para recuperar (probar y restaurar)" is "$(restarts)" 2

fresh; run start; age_marker $((25 * 3600)); touch "$work/vm/health-fails"; mid="$(cat "$env_file")"; run finish
check "finish con servicio enfermo falla" is "$status" 1
check "restaura la configuración con ambas claves" is "$(cat "$env_file")" "$mid"
check "conserva la marca original" test -e "$env_file.rotated-at"

fresh; printf 'APP_ENV=production\n' >"$env_file"; run start
check "sin clave actual se niega" is "$status" 1
check "explica el motivo" has "$output" "AGENT_TOKEN_ENCRYPTION_KEY falta"

fresh; printf 'APP_ENV=production\nAGENT_TOKEN_ENCRYPTION_KEY=corta\n' >"$env_file"; run start
check "con una clave inválida se niega" is "$status" 1

fresh; run explotar
check "acción desconocida se rechaza" is "$status" 1

printf '%d correctas, %d con fallo\n' "$passed" "$failed"
[[ "$failed" -eq 0 ]]
