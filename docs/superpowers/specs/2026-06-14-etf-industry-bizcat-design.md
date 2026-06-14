# ETF 산업 열에 대표 업종명 채우기 (bizcat)

- 작성일: 2026-06-14
- 범위: `internal/bizcat` (국내 매매일지 per-row 섹터/산업 보강)

## 배경

매매일지의 국내 거래 행에는 `섹터(E)` / `산업(F)` 열이 있고, `internal/bizcat` 패키지가
종목코드로 KIS를 조회해 채운다(현재 흐름: `InquirePrice` + `SearchStockInfo`).

- 일반 종목: `섹터 = bstp_kor_isnm`(업종 한글명), `산업 = std_idst_clsf_cd_name`(표준산업분류).
- ETF: `섹터 = "ETF"`(긴 라벨을 #71에서 짧게 정규화), `산업 = ""`(표준산업분류가 비어 옴).

문제: ETF 행은 섹터가 일률적으로 "ETF"이고 산업이 비어, 어떤 섹터를 추종하는 ETF인지
매매일지에서 알 수 없다.

## 목표

ETF로 판별된 종목에 한해, 비어 있는 **산업 열**에 ETF가 추종하는 대표 업종명을 채운다.
섹터 열의 "ETF" 라벨은 그대로 유지한다(ETF 식별자 보존, 정보 손실 없음).

- 예: KODEX 반도체 → `섹터 = "ETF"`, `산업 = "반도체"`

## 데이터 소스

KIS SDK `domestic.InquireEtfPrice`(ETF/ETN 현재가, FHPST02400000)의
`Output.EtfRprsBstpKorIsnm`(ETF 대표 업종 한글명).

## 설계

### 1. ETF 판별

`SearchStockInfo` 응답의 `EtfDvsnCd != ""` 로 판별한다(일반 종목은 빈 값).
기존 `bstp_kor_isnm` 접두사 휴리스틱보다 견고하다.

### 2. 조회 흐름 (`kisFetch` 클로저)

- 공통: `InquirePrice` + `SearchStockInfo` 호출(기존과 동일).
- ETF인 경우에만 3번째 호출 `InquireEtfPrice{Symbol: code}` 추가:
  - `섹터 = "ETF"` (유지)
  - `산업 = Output.EtfRprsBstpKorIsnm` (신규)
- 일반 종목: 호출 수·동작 변화 없음.
- `InquireEtfPrice` 실패 시: 산업만 빈 값으로 두고 섹터="ETF"는 채운다(전체 실패로 만들지 않음).

### 3. 캐시 자가 치유

기존 `config/bizcat_cache.json`에 ETF가 `{sector:"ETF", industry:""}`로 이미 캐시돼 있어,
그대로면 새 로직이 적용되지 않는다.

`Resolve`에서 캐시 히트라도 `Sector == "ETF" && Industry == ""`이면 미스로 간주해 재조회한다.
한 번 채워지면 이후 캐시를 사용한다. (별도 마이그레이션/캐시 버전 불필요)

엣지: 대표 업종명이 실제로 빈 ETF는 매 실행 재조회된다(산업이 계속 ""). 개인 매매일지의
ETF 코드 수가 적어(보통 한 자릿수) 비용이 무시할 만하므로 허용한다.

### 4. 부수 사항

- `kisCallsPerSec` 주석: "코드당 2회(ETF 3회)"로 갱신. 값 4는 유지.
- `--backfill-sectors`(#73)는 동일 `Resolve`를 사용하므로 ETF 산업까지 자동 백필된다(코드 변경 없음).

## 테스트

- `extractSectorIndustry` 및 fetch 분기를 ETF / 일반 두 케이스로 단위 테스트(fetch는 주입형 stub).
- 캐시 자가 치유: `{sector:"ETF", industry:""}` 캐시 항목이 재조회를 트리거하는지 검증.
- 기존 `bizcat` 테스트가 그대로 통과하는지 확인(`go test ./...`).

## 범위 밖 (YAGNI)

- ETF 구성종목 가중집계로 섹터 추론.
- 해외 ETF.
- 섹터 열 결합 표기("ETF·반도체").
