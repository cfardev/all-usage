# all-usage

El uso de tus suscripciones de Codex, Kiro y Cursor de un vistazo, en la terminal.

```
 ◆ all-usage                                         ↻ 56s · updated 21:06

╭────────────────────────────────────────────────────────────────────────╮
│ ● Codex                                                           Plus │
│                                                                        │
│ 5-hour limit                                                       34% │
│ ━━━━━━━━━━━━━━━━━━━━━━━╸────────────────────────────────────────────── │
│ resets in 2h 14m                                                       │
│                                                                        │
│ Weekly limit                                                       61% │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╸─────────────────────────── │
│ resets in 3d 5h                                                        │
│                                                                        │
│ chatgpt.com API · just now                                             │
╰────────────────────────────────────────────────────────────────────────╯

╭────────────────────────────────────────────────────────────────────────╮
│ ● Kiro                                                        Kiro Pro │
│                                                                        │
│ Credits                                                          41.6% │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╸──────────────────────────────────────── │
│ 416.11 / 1,000 credits                                                 │
│ resets in 27d                                                          │
│                                                                        │
│ kiro-cli login · just now                                              │
╰────────────────────────────────────────────────────────────────────────╯

╭────────────────────────────────────────────────────────────────────────╮
│ ● Cursor                                                           Pro │
│                                                                        │
│ Total usage                                                      72.4% │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╸─────────────────── │
│ resets in 12d                                                          │
│ $14.48 spent                                                           │
│                                                                        │
│ Auto + Composer                                                  58.3% │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╸───────────────────────────── │
│ resets in 12d                                                          │
│                                                                        │
│ API models                                                       91.2% │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╸────── │
│ resets in 12d                                                          │
│                                                                        │
│ Cursor IDE login · just now                                            │
╰────────────────────────────────────────────────────────────────────────╯

 r refresh • c compact • t theme • u used/left • ? more keys • q quit
```

<sub>Datos de ejemplo, tema `mono`. En la terminal los medidores se pintan de verde, amarillo o rojo según el uso.</sub>

- No necesita configuración: encuentra solo los logins de las apps oficiales (Codex CLI, kiro-cli, Kiro IDE, Cursor IDE y cursor-agent). En WSL también encuentra los de las apps de Windows.
- Muestra en una pantalla las ventanas de 5 horas y semanal de Codex, los créditos de Kiro y el uso incluido de Cursor, con cuánto falta para que se reinicie cada límite.
- Dashboard con auto-refresh, 10 temas, modo compacto, % usado o restante y layout que se adapta al ancho.
- Salidas para scripts: tabla, una línea para barras de estado, JSON y plantillas Go.
- Configurable con un archivo TOML comentado, variables de entorno y flags.
- Solo lectura: nunca renueva ni modifica tus credenciales.

## Instalación

Requiere Go 1.24 o superior.

```sh
go build -o ~/.local/bin/all-usage .

# con número de versión
go build -ldflags "-s -w -X github.com/cfardev/all-usage/internal/buildinfo.Version=v0.1.0" \
  -o ~/.local/bin/all-usage .
```

Cuando el repositorio esté publicado en `github.com/cfardev/all-usage`:

```sh
go install github.com/cfardev/all-usage@latest
```

Es Go puro (sin CGO), así que compila para otras plataformas con `GOOS`/`GOARCH`, por ejemplo `GOOS=darwin GOARCH=arm64 go build .`. Está probado en Linux y WSL. En macOS y Windows compila y busca las credenciales en sus rutas habituales, pero todavía no se ha probado.

## Uso

```sh
all-usage                  # dashboard en vivo
all-usage -p codex,kiro    # solo esos proveedores, en ese orden
all-usage -r 30s           # refrescar cada 30 s (0 desactiva el auto-refresh)
all-usage --theme nord     # otro tema (all-usage themes muestra todos)
all-usage show             # imprimir una vez y salir
all-usage doctor           # qué se detectó y cómo arreglar problemas
```

### Dashboard

| Tecla | Acción |
|---|---|
| `r` / `F5` | Refrescar ahora |
| `c` | Modo compacto |
| `t` | Cambiar de tema |
| `u` | Alternar % usado / % restante |
| `↑` `↓` / `k` `j`, `PgUp` `PgDn`, `Ctrl+u` `Ctrl+d` | Desplazarse cuando no cabe en pantalla |
| `?` | Ver todas las teclas |
| `q` / `Esc` / `Ctrl+c` | Salir |

Las tarjetas se acomodan en tantas columnas como quepan. El borde se pone amarillo o rojo cuando un medidor pasa `warn_at` o `critical_at`. Si un refresco falla, se conservan los últimos datos y se muestra el error. Si los datos no son en vivo, la tarjeta indica su antigüedad (`as of …`).

Modo compacto (`c` o `--compact`):

```
 ◆ all-usage                                         ↻ 55s · updated 21:06

╭────────────────────────────────────────────────────────────────────────╮
│ ● Codex                                                           Plus │
│ 5-hour limit    ━━━━━━━━━━━━━━╸───────────────────────────   34% 2h14m │
│ Weekly limit    ━━━━━━━━━━━━━━━━━━━━━━━━━╸────────────────   61%  3d5h │
│                                                                        │
│ chatgpt.com API · just now                                             │
╰────────────────────────────────────────────────────────────────────────╯

╭────────────────────────────────────────────────────────────────────────╮
│ ● Kiro                                                        Kiro Pro │
│ Credits         ━━━━━━━━━━━━━━━━━╸──────────────────────── 41.6%   27d │
│                                                                        │
│ kiro-cli login · just now                                              │
╰────────────────────────────────────────────────────────────────────────╯

╭────────────────────────────────────────────────────────────────────────╮
│ ● Cursor                                                           Pro │
│ Total usage     ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╸─────────── 72.4%   12d │
│ Auto + Composer ━━━━━━━━━━━━━━━━━━━━━━━━╸───────────────── 58.3%   12d │
│ API models      ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╸─── 91.2%   12d │
│                                                                        │
│ Cursor IDE login · just now                                            │
╰────────────────────────────────────────────────────────────────────────╯

 r refresh • c compact • t theme • u used/left • ? more keys • q quit
```

### Sin TUI: `all-usage show`

Imprime el uso una vez. Es lo que hace `all-usage` cuando su salida no es una terminal.

```
● Codex  Plus  chatgpt.com API · 0.4s
  5-hour limit     ██████▊░░░░░░░░░░░░░    34%  resets in 2h 14m
  Weekly limit     ████████████▎░░░░░░░    61%  resets in 3d 5h

● Kiro  Kiro Pro  kiro-cli login · 0.3s
  Credits          ████████▍░░░░░░░░░░░  41.6%  416.11 / 1,000 credits · resets in 27d

● Cursor  Pro  Cursor IDE login · 0.3s
  Total usage      ██████████████▌░░░░░  72.4%  resets in 12d · $14.48 spent
  Auto + Composer  ███████████▋░░░░░░░░  58.3%  resets in 12d
  API models       ██████████████████▎░  91.2%  resets in 12d
```

| Opción | Salida |
|---|---|
| `-f table` (default) | La tabla de arriba |
| `-f short` | Una línea para barras de estado: `Codex 5h 34% · wk 61% \| Kiro 41.6% \| Cursor 72.4%` |
| `--json` | JSON para scripts |
| `-t '<plantilla>'` | Formato propio con una plantilla de Go (`text/template`) |

En `short`, un `*` marca datos que no son en vivo y `✗` un proveedor que no se pudo leer. `show` termina con código 1 si no pudo leer ningún proveedor, y `doctor` si falla alguno.

Las plantillas reciben `.Now` y `.Providers`. Cada proveedor tiene `.ID`, `.Name`, `.Plan`, `.Account`, `.Source`, `.OK`, `.Stale`, `.Error`, `.Hint`, `.Meters` y `.Headline` (sus medidores principales). Cada medidor tiene `.ID`, `.Label`, `.Short`, `.Value`, `.Detail`, `.Percent`, `.Used`, `.Limit`, `.Unit` y `.ResetsAt`. Funciones disponibles: `pct`, `used`, `left`, `reset`, `bar <medidor> <ancho>`, `money`, `num`, `upper`, `lower` y `join`.

```sh
all-usage show -t '{{range .Providers}}{{.Name}}{{range .Headline}} {{pct .}}{{end}}  {{end}}'
# Codex 34% 61%  Kiro 41.6%  Cursor 72.4%

all-usage show --json | jq -r '.providers[] | select(.ok) | "\(.name): \(.usage.meters[0].percent // 0 | floor)%"'
# Codex: 34%
# Kiro: 41%
# Cursor: 72%
```

### Autocompletado

```sh
source <(all-usage completion bash)    # o zsh; para fish: all-usage completion fish | source
```

`all-usage completion <shell> --help` explica cómo dejarlo instalado para siempre.

## Configuración

Todo es opcional. `all-usage init` crea el archivo con todas las opciones comentadas y sus valores por defecto:

```sh
all-usage init          # crea ~/.config/all-usage/config.toml
all-usage config edit   # lo abre en $VISUAL/$EDITOR y lo valida al guardar
all-usage config show   # configuración efectiva (defaults + archivo + entorno + flags)
all-usage config path   # dónde está el archivo
all-usage themes        # vista previa de los temas
```

El archivo vive en `$XDG_CONFIG_HOME/all-usage/config.toml` (`~/.config/all-usage/config.toml`, también en macOS) o en `%APPDATA%\all-usage\config.toml` en Windows. Se puede usar otro con `-c` o `ALL_USAGE_CONFIG`.

Prioridad: valores por defecto < archivo < variables `ALL_USAGE_*` < flags. Las claves desconocidas generan una advertencia. Los valores inválidos dan un error que lista todos los problemas a la vez.

Ejemplo:

```toml
refresh_interval = "30s"
order = ["cursor", "codex", "kiro"]

[ui]
theme = "catppuccin"
percent = "remaining"   # lo que te queda en vez de lo usado
reset_format = "both"   # "in 2h 5m · 14:00"
warn_at = 60.0
critical_at = 85.0

[ui.colors]
accent = "#FF79C6"

[codex]
meters = ["primary", "secondary"]   # oculta créditos y límites por modelo

[kiro]
display_name = "Kiro (trabajo)"

[cursor]
meters = ["total"]   # solo el total; con enabled = false se oculta del todo
```

### Opciones generales e interfaz

| Clave | Default | Descripción |
|---|---|---|
| `refresh_interval` | `"1m"` | Cada cuánto se refresca el dashboard (`"30s"`, `"5m"`). `0` lo desactiva; mínimo 5 s. |
| `timeout` | `"20s"` | Tiempo máximo por proveedor en cada refresco. |
| `order` | `["codex", "kiro", "cursor"]` | Orden de las tarjetas. |
| `scan_windows` | `true` | En WSL, buscar también los logins de las apps de Windows. |
| `windows_home` | `""` | Perfil de Windows a usar en vez de detectarlo (por ejemplo `/mnt/c/Users/ana`). |
| `ui.theme` | `"auto"` | `auto`, `dark`, `light`, `dracula`, `nord`, `catppuccin`, `gruvbox`, `tokyonight`, `material-ocean`, `mono`. |
| `ui.layout` | `"auto"` | `auto` (las columnas que quepan), `columns` (una fila), `rows` (una columna). |
| `ui.columns` | `0` | Número fijo de columnas (0 = automático). |
| `ui.card_width` | `36` | Ancho mínimo de cada tarjeta en el layout automático. |
| `ui.compact` | `false` | Una línea por medidor. |
| `ui.bar_style` | `"blocks"` | `blocks`, `line`, `ascii`, `dots`. |
| `ui.percent` | `"used"` | `used` o `remaining`. |
| `ui.reset_format` | `"relative"` | `relative` (`in 2h 5m`), `absolute` (`Tue 14:00`) o `both`. |
| `ui.clock` | `"24h"` | `24h` o `12h`. |
| `ui.warn_at` / `ui.critical_at` | `70` / `90` | % usado a partir del cual el medidor se pone amarillo / rojo. |
| `ui.show_source` | `true` | Mostrar de dónde vienen los datos y su antigüedad. |
| `ui.show_account` | `false` | Mostrar el email de cada cuenta. |
| `ui.show_help` | `true` | Mostrar la línea de teclas al pie. |
| `ui.color` | `"auto"` | `auto` (solo en terminales), `always` o `never`. Respeta `NO_COLOR`. |
| `ui.colors.*` | `""` | Sobrescribe colores del tema: `accent`, `text`, `muted`, `border`, `ok`, `warn`, `critical`, `bar_empty`. Acepta `"#RRGGBB"`, `"#RGB"` o un índice ANSI `"0"`–`"255"`. |

### Opciones por proveedor

Las secciones `[codex]`, `[kiro]` y `[cursor]` comparten estas claves:

| Clave | Default | Descripción |
|---|---|---|
| `enabled` | `true` | Mostrar el proveedor. |
| `display_name` | `"Codex"`, `"Kiro"`, `"Cursor"` | Nombre en la tarjeta. |
| `color` | `"#10A37F"`, `"#9D6CFF"`, `"#4C9AFF"` | Color del punto de la tarjeta. |
| `meters` | `[]` | IDs de los medidores a mostrar, en ese orden (vacío = todos). |
| `source` | `"auto"` | De dónde leer los datos (ver abajo). |

Y estas son propias de cada uno:

| Clave | Default | Descripción |
|---|---|---|
| `codex.source` | `"auto"` | `auto` prueba la API con tu login, luego `codex app-server` y luego el último log de sesión. Para forzar una fuente: `api`, `app-server` o `sessions`. |
| `codex.home` | `""` | Carpeta de Codex (por defecto `$CODEX_HOME` o `~/.codex`). |
| `codex.binary` | `"codex"` | Ejecutable de Codex. |
| `codex.use_app_server` | `true` | Usar `codex app-server` cuando la API falla. |
| `codex.sessions_fallback` | `true` | Mostrar el último dato de los logs de sesión si no hay datos en vivo. |
| `codex.base_url` | `"https://chatgpt.com/backend-api"` | URL de la API. |
| `kiro.source` | `"auto"` | `auto` prueba kiro-cli y luego el Kiro IDE. Para forzar uno: `cli` o `ide`. |
| `kiro.db_path` | `""` | Base de datos de kiro-cli. |
| `kiro.ide_token_file` | `""` | Token del Kiro IDE. |
| `kiro.binary` | `"kiro-cli"` | Ejecutable de kiro-cli. |
| `kiro.refresh_with_cli` | `true` | Ejecutar `kiro-cli whoami` para que renueve un token vencido. |
| `kiro.region`, `kiro.endpoint`, `kiro.profile_arn` | `""` | Overrides. Por defecto salen de tu perfil de Kiro. |
| `cursor.source` | `"auto"` | `auto` prueba el Cursor IDE y luego cursor-agent. Para forzar uno: `ide` o `cli`. |
| `cursor.state_db` | `""` | Base de datos del IDE (`state.vscdb`). |
| `cursor.auth_file` | `""` | Login de cursor-agent. |
| `cursor.api_base`, `cursor.web_base` | `"https://api2.cursor.sh"`, `"https://cursor.com"` | URLs de las APIs. |

IDs de medidores para `meters` (`all-usage show --json` muestra el `id` de cada uno):

| Proveedor | IDs |
|---|---|
| `codex` | `primary` (5 horas), `secondary` (semanal), `credits` y, para límites extra como los de un modelo, `<limit_id>.primary` / `<limit_id>.secondary` |
| `kiro` | `credit`, `credit.bonus`, `credit.free_trial`, `credit.overage` |
| `cursor` | `total`, `auto`, `api`, `on_demand`, `team_on_demand`, `requests` |

### Variables de entorno

| Variable | Efecto |
|---|---|
| `ALL_USAGE_CONFIG` | Ruta del archivo de configuración. |
| `ALL_USAGE_PROVIDERS` | Proveedores a mostrar, en orden: `codex,cursor`. |
| `ALL_USAGE_REFRESH` | Intervalo de refresco: `30s`. |
| `ALL_USAGE_TIMEOUT` | Tiempo máximo por proveedor. |
| `ALL_USAGE_THEME` | Tema. |
| `ALL_USAGE_WINDOWS_HOME` | Perfil de Windows a usar desde WSL. |
| `ALL_USAGE_CODEX_TOKEN` (+ `ALL_USAGE_CODEX_ACCOUNT_ID`) | Usar ese token de Codex en vez de buscarlo. |
| `ALL_USAGE_KIRO_TOKEN` (+ `ALL_USAGE_KIRO_PROFILE_ARN`) | Usar ese token de Kiro en vez de buscarlo. |
| `ALL_USAGE_CURSOR_TOKEN` | Usar ese token de Cursor en vez de buscarlo. |
| `NO_COLOR` | Desactiva los colores. |

## De dónde salen los datos

Codex
- Credenciales: `auth.json` en `$CODEX_HOME` o `~/.codex`, y desde WSL también en `/mnt/c/Users/<usuario>/.codex`.
- Fuente: `GET https://chatgpt.com/backend-api/wham/usage`, el endpoint que usa el propio Codex para sus límites.
- Si el login venció, consulta a `codex app-server`, que renueva su propio login. Si tampoco puede, muestra el último dato guardado en `~/.codex/sessions`, marcado como desactualizado.
- Muestra la ventana de 5 horas, la semanal, los límites adicionales (por ejemplo por modelo) y los créditos.

Kiro
- Credenciales: la base de datos de kiro-cli (`~/.local/share/kiro-cli/data.sqlite3`; en macOS, `~/Library/Application Support/kiro-cli/`) o el token del Kiro IDE (`~/.aws/sso/cache/kiro-auth-token.json`).
- Fuente: `GET https://q.<región>.amazonaws.com/getUsageLimits`, la API de límites de uso de Kiro.
- Los tokens de kiro-cli duran alrededor de una hora. Si está vencido, ejecuta `kiro-cli whoami`, con lo que el cliente oficial lo renueva, y lo vuelve a leer.
- Muestra los créditos del mes (usados / límite) y su reinicio. También bonos, prueba gratuita y excedentes cuando existen.

Cursor
- Credenciales: la base de datos del Cursor IDE (`state.vscdb` en `~/.config/Cursor/User/globalStorage/`, `~/Library/Application Support/Cursor/User/globalStorage/` o `%APPDATA%\Cursor\User\globalStorage\`; desde WSL, la de Windows) o el login de cursor-agent (`~/.config/cursor/auth.json`). La base se abre en solo lectura con consultas puntuales, así que la lectura es rápida aunque pese varios GB.
- Fuente: `https://cursor.com/api/usage-summary`. Si falla, usa la API del IDE (`api2.cursor.sh`, `DashboardService`).
- Muestra el porcentaje del uso incluido en el ciclo de facturación (total, Auto + Composer y modelos por API), el gasto, el on-demand y, en planes antiguos, las requests premium.

Privacidad y seguridad
- Solo lectura: nunca escribe, renueva ni copia credenciales. Renovar un token por su cuenta podría invalidar el refresh token y cerrarte la sesión en la app oficial, así que la renovación se delega a los clientes oficiales.
- Cada token solo se envía al servicio que lo emitió (OpenAI, AWS o Cursor). No hay telemetría ni otros servidores.
- `doctor` nunca imprime tokens. Los emails se ocultan salvo con `--show-account` o `ui.show_account = true`.

## Barras de estado

tmux:

```tmux
set -g status-interval 60
set -g status-right '#(all-usage show -f short)'
```

Waybar:

```json
"custom/ai-usage": {
  "exec": "all-usage show -f short",
  "interval": 120
}
```

## Solución de problemas

- Empieza por `all-usage doctor`: lista cada login encontrado, dice si está vencido y prueba cada proveedor.
- `Session expired`: vuelve a iniciar sesión con `codex login` o `kiro-cli login`, o abre Cursor o usa `cursor-agent login`, según el proveedor.
- Codex muestra `as of …` o un `*`: tu login de Codex venció y estás viendo el último dato de los logs de sesión. `codex login` lo arregla.
- WSL: los perfiles de `/mnt/c/Users` se detectan solos. Para fijar uno usa `windows_home` (o `ALL_USAGE_WINDOWS_HOME`); para desactivarlo, `scan_windows = false`.
- Colores: cuando la salida no es una terminal se omiten. `ui.color = "always"` los fuerza y `--no-color` o `NO_COLOR` los quita.

## Desarrollo

```sh
go test -race ./...
go vet ./...
gofmt -l .
```

```
main.go
internal/
  cli/        comandos (cobra)
  config/     esquema, valores por defecto y plantilla comentada
  core/       modelo común: medidores, uso, errores y fetch concurrente
  providers/  codex/, kiro/ y cursor/
  tui/        dashboard (Bubble Tea + Lip Gloss)
  render/     salidas table, short, json y template
  ui/         temas, barras y formato
  sys/        rutas por sistema, WSL, SQLite de solo lectura y JWT
```

Para agregar un proveedor:

1. Implementa `core.Provider` (`ID`, `Name`, `Fetch` y `Doctor`) en `internal/providers/<nombre>`.
2. Regístralo en `internal/providers/providers.go`.
3. Agrega su sección en `internal/config`: el struct, los valores por defecto, `AllProviders`, `Common`, `SelectProviders`, `Validate` y la plantilla. Un test falla si la plantilla no coincide con los valores por defecto.

## Aviso

all-usage no es un producto oficial ni está afiliado a OpenAI, AWS, Kiro ni Cursor. Usa APIs internas sin documentar que pueden cambiar sin aviso.
