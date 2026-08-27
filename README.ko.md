# AgentLine

[![test](https://github.com/seongyooo/agentline/actions/workflows/test.yml/badge.svg)](https://github.com/seongyooo/agentline/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/seongyooo/agentline.svg)](https://pkg.go.dev/github.com/seongyooo/agentline)
[![MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**한국어** · [English](README.md)

AI 코딩 에이전트를 지켜보는 터미널 UI입니다.

한눈에 네 가지를 알려줍니다.

1. 에이전트가 지금 무엇을 하고 있나
2. 프로젝트의 어디에서 작업하고 있나
3. 무엇을 하라고 시켰고, 얼마나 진행됐나
4. 내가 봐줘야 하나

에디터도, Git 클라이언트도, Claude Code를 흉내 낸 것도 아닙니다. **에이전트가 무엇을 하는지**를 보여줄 뿐, 그 대상인 코드를 보여주지는 않습니다.

---

## 화면

100×28에서 실제로 렌더된 화면입니다(색은 제거).

```text
AGENTLINE   asks first                                                             ● CLAUDE  WORKING
                                                                                                    
╭─ PROJECT ◂ ────────────────────────╮ ╭─ AGENT · Opus 5 ──────────────────────────────────────────╮
│ ▾ Assets/                        ● │ │ MISSION                                                   │
│ ├─ ▾ Scripts/                    ● │ │ Add Git awareness to the header                           │
│ │  ├─ ▾ Core/                      │ │ ████████████░░░░░░░░░░░░ 2/4                              │
│ │  ├─ ▾ Player/                    │ │                                                           │
│ │  ├─ ▾ Puzzle/                  ● │ │ NOW                                                       │
│ │  │  ├─   Valve.cs              · │ │ Editing   3s                                              │
│ │  │  └─   DrainSystem.cs        ● │ │ .../Puzzle/DrainSystem.cs                                 │
│ │  └─ ▾ Rooms/                   ◐ │ │                                                           │
│ │     └─   WaterRoom.cs          ◐ │ │ Context ███████░░░░░░░░░░░░░░░░░  31%                     │
│ ├─ ▾ Prefabs/                      │ │ 5h      ██████████████░░░░░░░░░░  62%  resets 8/27 19:40  │
│ ├─ ▾ Scenes/                       │ │ 7d      ██████░░░░░░░░░░░░░░░░░░  28%  resets 8/31 13:00  │
╰───────────────────────────── 1/12 ─╯ ╰───────────────────────────────────────────────────────────╯
╭─ ACTIVITY ───────────────────────────────────────────────────────────────────────────────────────╮
│ 00:26  Reading   .../Puzzle/Valve.cs                                                             │
│ 00:26  Editing   .../Rooms/WaterRoom.cs                                                          │
│ 00:26  Running   git status --porcelain                                                          │
│ 00:26  Reading   .../Player/Move.cs                                                              │
│ 00:28  Reading   .../Rooms/WaterRoom.cs                                                          │
│ 00:29  Running   Run the git package tests                                                       │
│ 00:30  Done      Run the git package tests                                                       │
│ 00:31  Editing   .../Puzzle/DrainSystem.cs                                                       │
╰─────────────────────────────────────────────────────────────────────────────────────── 43-50/50 ─╯
PULSE  █▆▆█▆▆█▆▆█▆▁     ▄▆█▆▆█▆▆█▆▆█▆▆█    ▄▆█▆▆█▆▆█▆▆█▆▆▆      ▁  ▁  ▁ ▁                00:09 → now
                                                                                                    
> Ask claude...                                                   enter inspect   tab focus   q quit
```

- **헤더** — 에이전트와 상태(`WORKING`, `WAITING`, `NEEDS INPUT`, `DONE`, `ERROR`), 그리고 권한 모드. 모드는 에이전트가 묻지 않고 할 수 있는 범위에 따라 색이 다릅니다.
- **PROJECT** — 파일 트리. `▾`/`▸`로 펼친 폴더와 접힌 폴더를 구분하고, 에이전트가 건드린 파일과 폴더에 표시가 붙습니다(`●` 지금, `◐` 옅어지는 중). 건드린 파일은 알아서 펼쳐지고, 쓰겠다고 예고만 하고 아직 쓰지 않은 파일은 실제로 만들어질 때까지 흐리게 나옵니다.
- **MISSION / NOW / NEXT / REPLY** — 프롬프트에서 뽑아낸 목표, 지금 하는 일과 걸린 시간(에이전트가 스스로 붙인 설명이 있으면 그것으로), 그리고 답변이 담긴 스크롤 가능한 칸. `MISSION` 아래 막대는 **에이전트가 직접 관리하는 작업 목록**을 세며, 목록을 안 쓰면 아예 나오지 않습니다. AgentLine이 완료율을 추측하는 일은 없습니다.
- **세션 상태** — 컨텍스트가 얼마나 찼는지, 각 사용량 한도가 얼마나 남았는지를 막대와 숫자로 함께, 칸 맨 아래에 고정해서 보여줍니다. 모델 이름은 패널 제목이 됩니다. 전부 에이전트가 보고한 값이고 측정하거나 추정한 것은 없습니다. 에이전트가 말한 적 없는 한도는 줄 자체가 없습니다. 여유 행이 없는 칸에서는 막대가 물러나고 같은 숫자를 글자로 보여줍니다.
- **NEEDS YOU** — 에이전트가 권한을 물으며 멈추면 그 질문이 칸 맨 위를 차지하고 터미널이 울립니다. 벨과 OSC 9 데스크톱 알림을 함께 보내며, Windows Terminal과 iTerm2는 실제 알림을 띄우고 모르는 터미널은 조용히 무시합니다. `y` · `n`으로 답하고, `a`는 허용과 동시에 에이전트가 제안한 모드로 바꿉니다. AgentLine이 사용자를 방해하는 유일한 경우이고, 나머지가 조용할 수 있는 이유이기도 합니다. AgentLine이 소유한 세션(`--run`)에서만 됩니다. 밖에서 지켜보기만 하면 질문이 애초에 이쪽으로 오지 않습니다.
- **ACTIVITY** — 최근 활동 기록. 시간이 지나면 흐려집니다.
- **PULSE** — 세션 전체를 한 줄로. 막대 높이는 그 시간 조각에 떨어진 행동 수, 색은 그중 가장 눈에 띄는 종류, 빈칸은 실제로 조용했던 구간입니다. 로그와 별도로 세기 때문에 로그에서 이미 밀려난 작업도 남아 있습니다. 스크롤백이 될 수 없는 화면이 이겁니다 — 20분 자리 비우고 와서 한 번 훑으면 작업이 몰린 곳, 멈춘 곳, 깨진 곳이 보입니다.
- **입력줄** — AgentLine이 소유한 세션에 프롬프트를 보냅니다. Git 브랜치와 지금 쓸 수 있는 키도 함께 표시됩니다.

터미널 크기에 따라 접힙니다. 좁아지면 한 칼럼으로 바뀌고, `NOW`를 자르기 전에 우선순위가 낮은 것(`NEXT`, 그다음 `REPLY`)부터 버립니다.

---

## 설치

Go가 없어도 됩니다. 아래 명령이 빌드된 바이너리를 받아 옵니다.

**macOS · Linux**

```sh
curl -fsSL https://raw.githubusercontent.com/seongyooo/agentline/main/install.sh | sh
```

**Windows**

```powershell
irm https://raw.githubusercontent.com/seongyooo/agentline/main/install.ps1 | iex
```

[릴리스 페이지](https://github.com/seongyooo/agentline/releases)에서 직접 받아 `PATH`에 놓아도 됩니다.

<details>
<summary>Go로 설치하거나 소스에서 빌드하기</summary>

```sh
go install github.com/seongyooo/agentline/cmd/agentline@latest
```

Go 1.26 이상이 필요하고, `$(go env GOPATH)/bin`에 설치됩니다.

직접 고쳐 쓰려면:

```sh
git clone https://github.com/seongyooo/agentline
cd agentline
go build ./cmd/agentline
```

</details>

---

## 사용법

Claude Code를 지켜보는 방법이 두 가지 있고, 돈을 쓰지 않고 화면만 구경하는 방법이 하나 더 있습니다.

### 1. AgentLine이 세션을 직접 띄우기 (권장)

```sh
agentline --run
```

`claude`를 스트리밍 JSON 모드로 띄우고 프로세스를 계속 살려 둡니다. 그래서 입력창이 살아 있고, 프롬프트를 보낼 때마다 같은 대화가 이어집니다. 프로젝트에 뭘 설치할 필요도, 끝난 뒤 남는 것도 없습니다. `claude` 실행 파일이 `PATH`에 있어야 합니다.

### 2. 내가 띄운 Claude Code 세션을 옆에서 보기

기본적으로 `127.0.0.1:8787`에서 Claude Code hook을 기다립니다. 지켜볼 프로젝트에 hook 설정을 넣어 주세요.

```sh
agentline --print-hooks          # 출력을 .claude/settings.json에 합칩니다
agentline                        # 그다음 평소처럼 Claude Code를 씁니다
```

설정은 문서로 적어 두지 않고 **매번 만들어 냅니다.** 그래야 AgentLine이 실제로 듣고 있는 주소와 어긋날 수 없습니다. 이 모드에서 입력창은 동작하지 않습니다. 세션의 주인은 당신 터미널이지 AgentLine이 아니니까요.

### 3. 돈 쓰지 않고 써 보기

```sh
go build -o fakeagent ./cmd/fakeagent
agentline --run --agent ./fakeagent --root /적당한/빈/폴더
```

`fakeagent`는 같은 파이프로 같은 스트리밍 JSON 프로토콜을 말합니다. 그래서 실제 어댑터도, 프로세스 처리도, 컨트롤 요청도 전부 진짜 경로를 탑니다. 빠진 건 생각하는 부분뿐입니다. 지정한 폴더의 파일을 진짜로 읽고 쓰므로 트리와 활동 기록이 돈 드는 세션과 똑같이 움직입니다. **파일을 하나 만들기 때문에 빈 폴더를 지정하세요.**

`agentline --source mock`도 있습니다. 아무것도 띄우지 않고 예시 활동만 재생하는데, 이 모드에서는 입력창이 동작하지 않습니다.

### 플래그

| 플래그 | 기본값 | 설명 |
|---|---|---|
| `--root` | 현재 폴더에서 탐지 | 프로젝트 루트 |
| `--run` | `false` | 세션을 직접 띄우고 소유 |
| `--agent` | `claude` | `claude` 대신 실행할 파일 |
| `--source` | `claude` | 어느 에이전트인가: `claude`, `codex`, `mock` |
| `--addr` | `127.0.0.1:8787` | hook을 받을 주소 |
| `--mission` | — | 프롬프트에서 뽑는 대신 `MISSION`을 고정 |
| `--notify` | — | 에이전트가 답을 기다릴 때 실행할 명령. 자체 알림이 없는 터미널용 |
| `--print-hooks` | — | 설치할 hook 설정을 출력하고 종료 |
| `--mock-interval` | `2s` | mock 이벤트 간격 |

진단 로그는 `<사용자 캐시 폴더>/agentline/agentline.log`로 갑니다. 화면에는 절대 찍지 않습니다.

### 키

스크롤되는 칸이 여럿이라 화살표 키는 한 번에 한 칸에만 갑니다. 지금 키가 가는 칸은 제목에 `◂`로 표시됩니다.

| 키 | 동작 |
|---|---|
| `tab` | 트리 · `REPLY` · `ACTIVITY` · 입력창 사이로 포커스 이동 |
| `↑` `↓` `pgup` `pgdn` | 포커스된 칸 스크롤 |
| `→` `←` | 폴더 펼치기 / 접기 (트리에서만) |
| `enter` | 선택한 파일이나 폴더 설명 (트리에서만) |
| `esc` | 설명이나 입력창에서 빠져나오기 |
| `i` | 입력창으로 바로 가기 |
| `y` `n` | 에이전트가 막혀 물어보는 것을 허용 / 거부 |
| `a` | 허용하면서 에이전트가 제안한 권한 모드로 전환 |
| `shift+tab` | 권한 모드 전환 |
| `ctrl+n` | 세션을 새로 시작해 쌓인 컨텍스트 비우기 |
| `q` / `ctrl+c` | 종료 |

클릭으로도 포커스가 옮겨 가고, 휠은 포인터 아래 있는 칸을 스크롤합니다.

입력창에서는 `enter`가 전송이고 나머지 키는 전부 글자입니다. `/`를 치면 세션이 알려 준 명령이 자동완성되고, `/model`은 선택기를 엽니다.

---

## 지원하는 에이전트

| 에이전트 | 방식 | 상태 |
|---|---|---|
| Claude Code | 세션 소유(stream-JSON) 또는 hook | 실제 세션으로 검증 |
| Codex | `codex exec --json` | 공개 스키마대로 구현. **실제 세션으로는 아직 미검증** |

```sh
agentline --run --source codex
```

Codex는 구조가 다릅니다. `codex exec`는 턴 하나를 처리하고 종료하기 때문에, AgentLine은 프로세스를 붙잡는 대신 **Codex가 알려 준 스레드 ID를 기억했다가 다음 프롬프트에서 이어받습니다.** 대화는 그대로 이어지지만 오래 사는 프로세스는 없습니다. 명령의 실제 종료 코드를 알려 주고, 패치가 파일을 추가했는지 수정했는지 삭제했는지도 구분해 주기 때문에 그 부분은 Claude 어댑터보다 정확하게 표시됩니다.

어댑터는 Codex가 공개한 이벤트 스키마에 맞춰 작성했고, 그 프로토콜을 말하는 대역으로 종단 검증까지 했습니다. 다만 **실제 바이너리에 물려 본 사람은 아직 없습니다.** 누군가 해 보기 전까지는 미검증으로 봐 주세요.

에이전트를 추가한다는 건 그 출력을 정규화된 이벤트 모델로 옮기는 어댑터 하나를 쓴다는 뜻입니다. 어댑터 아래쪽 코드는 지금 어느 에이전트를 보고 있는지 알지 못합니다.

에이전트를 추가할 값어치가 있으려면 **자기가 한 일을 구조화된 채널로 알려 줘야 합니다.** 터미널 출력을 긁어서 활동을 재구성하는 방식은 이 프로젝트가 피하려고 만들어진 것입니다. 멀쩡해 보이다가 출력 형식이 바뀌는 순간, 틀렸다는 말도 없이 틀리기 때문입니다.

---

## 구조

```text
Claude Code ──stream-json──┐
Claude Code hooks ──HTTP───┼──► internal/agent/... ──► events.Event ──► state ──► tui
Codex ──────────────JSONL──┘        (어댑터)            (정규화)       (리듀서)
```

| 패키지 | 역할 |
|---|---|
| `cmd/agentline` | 플래그, 루트 탐지, 배선, 로깅 |
| `internal/agent` | `Source` 이음새와 mock 백엔드 |
| `internal/agent/claude` | Claude Code 어댑터: hook 서버, 세션 소유, 변환 |
| `internal/agent/codex` | Codex 어댑터: 턴마다 프로세스 하나, 스레드 ID로 재개 |
| `internal/events` | 정규화된, 제공자 중립적인 이벤트 모델 |
| `internal/state` | 리듀서 — 상태가 바뀌는 유일한 곳 |
| `internal/project` | 지연 스캔 파일시스템, 트리 모델 |
| `internal/git` | 브랜치와 변경된 파일 조회 |
| `internal/tui` | Bubble Tea 모델, 레이아웃, 렌더링 |

이벤트 모델은 `file_read`, `file_edit`, `file_create`, `file_delete`, `file_pending`, `command_start`, `command_end`, `agent_status`, `agent_error`, `user_prompt`, `agent_reply`, `task_progress`, `session_info`를 다룹니다. **어댑터는 관측하지 못한 필드를 채우면 안 됩니다.** AgentLine은 본 것만 보고하고, 모르는 것에 대해서는 아무 말도 하지 않습니다.

세션을 소유하면 에이전트에게 컨트롤 요청도 보낼 수 있습니다. 권한 모드, 모델, 컨텍스트 사용률이 그렇습니다. 이 프로토콜은 공개된 SDK 문서에 없어서 CLI에서 직접 읽어냈고, 그래서 형식이 바뀌면 알려 주는 테스트로 고정해 뒀습니다.

[Bubble Tea](https://github.com/charmbracelet/bubbletea)와 [Lip Gloss](https://github.com/charmbracelet/lipgloss) 위에서 만들었습니다.

---

## AgentLine이 되지 않을 것

에디터, diff 뷰어, Git 클라이언트, 대화 기록 창, 토큰 사용량 대시보드가 되지 않습니다. 그런 도구는 이미 있고, 그 일은 그쪽이 더 잘합니다. `REPLY`가 답변을 한 줄만 보여주는 것도 같은 이유입니다. 턴이 끝났다는 걸 알기에 딱 그만큼입니다.

---

## 개발

```sh
go test ./...
```

일부 테스트는 실제 에이전트나 실제 터미널, 큰 트리가 필요해서 기본으로 돌지 않습니다.

| 테스트 | 켜는 방법 |
|---|---|
| 실제 Claude Code 세션과 hook | `LIVE_CLAUDE=<루트>`, `LIVE_ADDR` |
| 실행 중인 세션을 종단 관측 | `OBSERVE_ROOT=<루트>`, `OBSERVE_ADDR`, `OBSERVE_MISSION` |
| 프레임 단위 UI 미리보기 | `LIVE_PREVIEW=1` |
| 실제 트리에서 스캐너 벤치마크 | `BENCH_ROOT=<루트>` |

나머지는 에이전트 없이 돌아갑니다. 스트리밍 어댑터는 프로토콜을 말하는 대역으로 검증하므로, 프로토콜이 바뀌면 **아무도 안 보는 세션이 조용히 망가지는 대신 테스트가 깨집니다.**

`IMPLEMENTATION.md`가 설계 문서이자 범위의 기준입니다. `docs/hook-spike.md`에는 Claude Code hook이 실제로 무엇을 주는지 적혀 있습니다. 가정이 아니라 측정한 결과입니다.

---

## 앞으로

- **실제 Codex 바이너리 검증** — 어댑터는 작성했고 테스트도 있지만 대역으로만 돌려 봤습니다.
- **자리 비운 사이 요약** — 모양뿐 아니라 순효과까지.
- **Git 정보 확장** — 지금의 브랜치와 변경 파일 너머.
- **지능 레이어** — 관측된 이벤트에서 `NEXT`를 추론하고 단계를 요약.

---

## 기여

이 프로젝트가 터미널 IDE로 변질되지 않게 지키는 규칙입니다.

1. 한 번에 한 페이즈씩 만들 것. 모든 페이즈는 실행 가능해야 합니다.
2. 과설계하지 말 것. 두 번째 구현이 눈앞에 보이지 않으면 추상화하지 않습니다.
3. 특정 제공자에 종속된 코드는 그 어댑터 안에 둘 것.
4. UI와 비즈니스 로직을 섞지 말 것. 상태는 리듀서가 소유합니다.
5. 제품 철학을 지킬 것. 상세가 아니라 상황 인지입니다.
6. 이미 있는 도구를 다시 만들지 말 것.
7. 추정보다 관측된 사실을 택할 것. **모르면 아무 말도 하지 않습니다.**

PR을 열기 전에 `IMPLEMENTATION.md`를 읽고, `go test ./...`를 통과시켜 주세요. CI는 Linux · macOS · Windows에서 race detector를 켜고 테스트를 돌립니다. 경로 처리, 프로세스 처리, 터미널 폭 계산이 셋 다 플랫폼마다 달라서 전부 통과해야 합니다.

이슈와 PR 환영합니다. 다른 에이전트를 붙이려면 이음새는 `internal/agent.Source`이고, `internal/agent/claude`가 참고할 만한 예제입니다. `docs/hook-spike.md`에는 그 동작을 어떻게 확인했는지가 적혀 있습니다.

---

## 라이선스

[MIT](LICENSE)
