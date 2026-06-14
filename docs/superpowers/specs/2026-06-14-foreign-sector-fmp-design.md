# 해외(미국·일본) 종목 섹터/산업 채우기 (FMP)

- 작성일: 2026-06-14
- 범위: `internal/fmpcat`(신규), `cmd/atj/main.go`, `internal/writer/backfill.go`

## 배경

해외 시트(17컬럼)의 `섹터(F)`/`산업(G)` 열은 설계상 공란이다(`ToForeignRow` 빈 값,
`enrichSectors` 는 `IsDomestic` 만 처리, `BackfillSectors` 는 국내 헤더만 대상).
국내는 `internal/bizcat`(KIS)로 채워지지만 해외는 비어 있다.

## 데이터 소스 결정: FMP (`fmp-go`)

- KIS 해외 SDK: `InquirePriceDetail().EIcod`(업종 코드만) → 섹터/산업 분류 부적합.
- **FMP `Company.Profile(symbol)`** → `Sector`/`Industry`(영문) 반환. 검증 완료:
  - AAPL→Technology/Consumer Electronics, NVDA→Technology/Semiconductors,
    7203.T→Consumer Cyclical/Auto-Manufacturers, 0700.HK→Communication Services/...
- 비미국은 **거래소 접미사 필수**(`7203`→not found, `7203.T`→OK).
- 해외 ETF(`1321.T` 등)는 FMP 미커버(not found).

## 결정 사항

- **표기**: 섹터·산업 모두 **FMP 영문 원본** 그대로(추후 대시보드 나라별 비중 분석용).
- **범위(통화→거래소 접미사)**: 현재 **US/JP 만**. `USD→""`, `JPY→".T"`. 그 외 통화는 공란(추후 확장).
- **not-found/해외 ETF**: 공란(추후 필요 시 확장).

## 설계 (bizcat 대칭)

### internal/fmpcat 리졸버
```
Resolve(ticker, currency string) (sector, industry string)
```
- `exchangeSuffix(currency)`: `USD→("",true)`, `JPY→(".T",true)`, 그 외 `("",false)`.
  - 미지원 통화 → 즉시 공란 반환(FMP 호출 안 함).
- `symbol = ticker + suffix`, **캐시 키 = symbol**.
- `fetch(symbol)`: `FMP Company.Profile(symbol)` → `(Sector, Industry)`.
  - `errors.Is(err, fmp.ErrNotFound)` → `("","",nil)` 반환 → **빈 값으로 영구 캐시**(재조회 안 함; 해외 ETF/미커버).
  - 그 외 오류 → 에러 전파 → negative-cache(이번 실행만, 다음 실행 재시도).
- 영구 캐시 `config/fmpcat_cache.json`(lazy, 한글 없음). `FMP_API_KEY` 없으면 클라이언트 생성 실패 → no-op fetch(회복력).
- 비결정적 호출 아님(FMP 응답 안정) → 캐시 버전 불필요.

### enrichSectors 해외 분기 (`cmd/atj/main.go`)
```
enrichSectors(trades, bizcatRes, fmpRes)
  국내(IsDomestic, code≠"")  → bizcatRes.Resolve(code, name)
  해외(IsForeign, code≠"")   → fmpRes.Resolve(ticker=code, currency)
```
- `foreignResolver` 인터페이스(`Resolve(ticker, currency) (string,string)`) 추가(스텁 주입).
- processor 에 `fmpRes`/`fmpStore` 필드, `newProcessor` 에서 `fmpcat.New(...)` 조립 + `Save()` defer.

### BackfillSectors 해외 시트 지원 (`internal/writer/backfill.go`)
시트 헤더로 국내/해외 분기:
- `DomesticHeaders` → C:D(코드,종목명) 읽기, `domesticResolve(code,name)`, **E:F** 기록(기존).
- `ForeignHeaders` → C:D(통화,종목코드) 읽기, `foreignResolve(ticker=D, currency=C)`, **F:G** 기록.
- 그 외(구포맷/비매매일지) 스킵.
- 시그니처: `BackfillSectors(ctx, domesticResolve, foreignResolve)`.

## 테스트
- `exchangeSuffix`: USD→"", JPY→".T", 기타→미지원.
- `Resolve`: 미지원 통화→공란(fetch 미호출), USD 캐시 히트/미스, JPY 접미사 부착, not-found→빈 캐시(재조회 안 함), 에러→negative-cache 재시도.
- `backfillValues` 해외용(통화+티커 입력).
- `BackfillSectors` 헤더 분기(국내 E:F / 해외 F:G) — 가능 범위 단위 테스트.
- `enrichSectors` 해외 분기(스텁) + 기존 국내 유지.
- `go test ./...` 전체 통과.

## 범위 밖 (YAGNI)
- US/JP 외 통화(HK/CN/EU/VN 등) — 보유 시 확장.
- 해외 ETF 분류(OpenAI 재사용) — 추후.
- 대시보드 나라별 섹터/산업 비중 — 별도 작업.
