# ETF 산업 열 분류 채우기 (bizcat 하이브리드)

- 작성일: 2026-06-14
- 범위: `internal/bizcat`, `internal/writer/backfill.go`, `cmd/atj/main.go`

## 배경

매매일지 국내 거래 행의 `섹터(E)` / `산업(F)` 열을 `internal/bizcat` 이 KIS 조회로 채운다.
일반 종목은 섹터=업종 한글명(`bstp_kor_isnm`), 산업=표준산업분류(`std_idst_clsf_cd_name`).
ETF 는 섹터="ETF", 산업은 비어 있었다.

1차 시도(merged)로 ETF 산업에 `InquireEtfPrice.EtfRprsBstpKorIsnm`(대표 지수명)을 넣었으나,
이는 "반도체/종합" 같은 분류가 아니라 **추종 지수의 풀네임**("S&P 500", "Bloomberg World ...")이라
산업 분류로는 부적절했다.

## 실측 (보유 ETF 85개)

`EtfTrgtNmixBstpCode`(목표 지수 업종코드) 분포:
- **분류됨 17개(20%)**: 국내 시장/섹터 ETF. 코드→KRX 업종명(0001 종합, 4003 반도체, 4005 은행 …).
- **9999 미분류 68개(80%)**: 해외(S&P500·나스닥·니케이 …) + 국내 테마(2차전지·방위산업·AI …).
  KIS 가 한국 업종으로 분류하지 않음.

→ 업종코드 단독은 20%만 커버. 나머지 80%는 종목명 기반 분류가 필요.

## 목표

ETF 산업 열을 의미 있는 카테고리로 채운다. 섹터="ETF" 는 유지.

- 코드 분류 ETF: KRX 업종명(예 "반도체", "은행", "종합")
- 9999 ETF: 펀드 종목명 기반 OpenAI 분류(예 "미국주식", "방위·우주항공", "원자재")

## 설계 (하이브리드)

### 데이터 소스
`domestic.InquireEtfPrice` 의 `EtfTrgtNmixBstpCode` + `EtfRprsBstpKorIsnm`.

### ETF 산업 결정 (`resolveETFIndustry`, 순수 함수)
- `trgtCode != "9999"` → `stripKRXPrefix(rprsName)` (KRX 분류명, "KRX " 접두사 제거)
- `trgtCode == "9999"` → `classify(fundName)` (OpenAI ETF 분류기)
  - 분류기 미설정/실패 시 `stripKRXPrefix(rprsName)` 로 폴백(지수명, 회복력)

### ETF 전용 OpenAI 분류기 (`internal/bizcat/etfclass.go`, 신설)
기존 `internal/sector` 는 GICS 12섹터라 ETF(시장/자산군/테마)에 부적합 → 별도 분류기.

고정 taxonomy(19종):
```
미국주식, 중국주식, 일본주식, 인도주식, 베트남주식, 글로벌주식,
반도체, 2차전지, 바이오·헬스케어, AI·로봇, 신재생에너지, 원자력,
방위·우주항공, 배당, 리츠·부동산, 원자재, 채권, 통화·단기금리, 기타테마
```
- 입력: 펀드 종목명(예 "KODEX 미국S&P500"). 지수명보다 지역·테마가 명확.
- 단건 호출 + bizcat 영구 캐시(코드당 1회 분류 후 캐시). `STOCK_DATA_OPENAI_API_KEY` 없으면 분류기 nil.
- 검증: 응답이 taxonomy 밖이면 "기타테마".

### 종목명 배선
`Resolve(code)` → `Resolve(code, name string)`. ETF 9999 분류 입력으로 펀드명 전달.
- `enrichSectors`: `r.Resolve(t.StockCode, t.StockName)`
- `BackfillSectors`: 시트에서 C(코드)+D(종목명) 함께 읽어 전달

### 캐시 버전 (선별 갱신)
`entry` 에 `Version int` 추가. `needsRefresh(e) = e.Sector=="ETF" && e.Version<etfCacheVersion`.
- 기존 ETF 캐시(지수명 산업, version 0)는 1회 재조회되어 하이브리드 적용.
- 비-ETF 주식 캐시는 그대로 보존(전체 삭제 불필요).
- 재조회 실패 시 기존 캐시 보존(섹터 공백화 방지).

## 테스트
- `resolveETFIndustry`: 코드분류/9999+분류기/9999+분류기실패 폴백 3케이스.
- `stripKRXPrefix`: "KRX 반도체"→"반도체", "종합"→"종합".
- `needsRefresh`: 구버전 ETF(version 0) 재조회, 신버전/비-ETF 미재조회.
- etfclass 검증: taxonomy 밖 응답→"기타테마"(순수 검증 함수).
- backfill: 2-arg resolve 시그니처, 코드+종목명 전달.
- `go test ./...` 전체 통과.

## 범위 밖 (YAGNI)
- ETF 구성종목 가중집계. 해외 ETF 섹터(국내 시트 한정). 배치 OpenAI 호출(단건+캐시로 충분).
