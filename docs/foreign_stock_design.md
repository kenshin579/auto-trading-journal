# 해외 주식 거래 처리 기능 설계 문서

## 1. 개요

### 1.1 배경
현재 시스템은 국내 주식 거래만 처리 가능하며, 해외 주식 거래는 다른 데이터 구조를 가지고 있어 처리가 불가능합니다.

### 1.2 목표
- 국내/해외 주식 거래를 모두 처리할 수 있는 통합 시스템 구축
- 기존 코드의 확장성 보장
- 환율 정보 및 해외 거래 특성 반영

## 2. 데이터 구조 비교

### 2.1 국내 주식 (현재)
```
일자 | 종목명 | 매수(수량/평균단가/매수금액) | 매도(수량/평균단가/매도금액) | 매매비용 | 손익금액 | 수익률
```

### 2.2 해외 주식 (추가 필요)
```
매매일 | 통화 | 종목번호 | 종목명 | 잔고수량 | 환율정보 | 매수정보 | 매도정보 | 수수료 | 세금 | 손익정보
```

## 3. 주요 변경사항

### 3.1 클래스 구조 변경

#### 3.1.1 Trade 클래스 계층 구조
```python
# 기본 Trade 클래스 (추상 클래스)
class BaseTrade(ABC):
    date: str
    stock_name: str
    
    @abstractmethod
    def to_sheet_row(self) -> List[str]:
        pass
    
    @abstractmethod
    def validate(self) -> bool:
        pass

# 국내 주식용
class DomesticTrade(BaseTrade):
    trade_type: str  # 매수/매도
    quantity: int
    price: float
    amount: float
    fee: int
    profit: int
    profit_rate: float
    
    def to_sheet_row(self) -> List[str]:
        # 8개 컬럼 반환

# 해외 주식용
class ForeignTrade(BaseTrade):
    currency: str
    ticker: str
    balance_quantity: int
    buy_exchange_rate: float
    sell_exchange_rate: float
    buy_quantity: int
    buy_price: float
    buy_amount: float
    buy_amount_krw: float
    sell_quantity: int
    sell_price: float
    sell_amount: float
    sell_amount_krw: float
    commission: float
    tax: float
    total_cost_krw: float
    avg_buy_price_krw: float
    trading_profit: float
    trading_profit_krw: float
    fx_gain_loss: float
    total_profit: float
    profit_rate: float
    fx_profit_rate: float
    
    def to_sheet_row(self) -> List[str]:
        # 22개 컬럼 반환
```

### 3.2 FileParser 수정

#### 3.2.1 파일 타입 감지
```python
class FileParser:
    def detect_file_type(self, file_path: Path) -> str:
        """파일 타입 감지 (domestic/foreign)"""
        # 1. 파일명으로 감지
        if "해외" in file_path.name:
            return "foreign"
        elif "국내" in file_path.name:
            return "domestic"
        
        # 2. 헤더로 감지
        with open(file_path, 'r', encoding='utf-8') as f:
            header = f.readline().strip()
            if "통화" in header or "환율" in header:
                return "foreign"
            return "domestic"
    
    def parse_trading_log(self, file_path: Path) -> TradingLog:
        file_type = self.detect_file_type(file_path)
        
        if file_type == "domestic":
            return self._parse_domestic_log(file_path)
        else:
            return self._parse_foreign_log(file_path)
    
    def _parse_foreign_log(self, file_path: Path) -> TradingLog:
        """해외 주식 파일 파싱"""
        trades = []
        
        # 해외 주식은 한 행에 매수/매도 정보가 모두 있음
        # 매수와 매도를 분리하여 처리
        for row in rows:
            # 매수 거래 생성 (buy_quantity > 0)
            if row['buy_quantity'] > 0:
                buy_trade = ForeignTrade(
                    date=row['date'],
                    stock_name=row['stock_name'],
                    ticker=row['ticker'],
                    trade_type='매수',
                    # ... 기타 필드
                )
                trades.append(buy_trade)
            
            # 매도 거래 생성 (sell_quantity > 0)
            if row['sell_quantity'] > 0:
                sell_trade = ForeignTrade(
                    date=row['date'],
                    stock_name=row['stock_name'],
                    ticker=row['ticker'],
                    trade_type='매도',
                    # ... 기타 필드
                )
                trades.append(sell_trade)
```

### 3.3 SheetManager 수정

#### 3.3.1 시트 이름 패턴
```python
class SheetManager:
    def find_target_sheets(self, prefix: str, file_type: str = "domestic") -> Dict[str, Optional[str]]:
        """파일 타입에 따른 시트 찾기"""
        
        if file_type == "foreign":
            target_sheets = {
                "주식_매수": f"{prefix} 해외 주식 매수내역",
                "주식_매도": f"{prefix} 해외 주식 매도내역", 
                "ETF_매수": f"{prefix} 해외 ETF 매수내역",
                "ETF_매도": f"{prefix} 해외 ETF 매도내역"
            }
        else:
            # 기존 국내 주식 패턴
            target_sheets = {
                "주식_매수": f"{prefix} 주식 매수내역",
                "주식_매도": f"{prefix} 주식 매도내역",
                "ETF_매수": f"{prefix} ETF 매수내역", 
                "ETF_매도": f"{prefix} ETF 매도내역"
            }
```

### 3.4 StockClassifier 수정

#### 3.4.1 해외 종목 분류
```python
class StockClassifier:
    # 해외 ETF 패턴
    FOREIGN_ETF_PATTERNS = [
        r'^[A-Z]{2,5}$',  # 2-5자리 대문자 티커
        'ETF', 'FUND', 'TRUST', 'SHARES', 'SPDR', 'ISHARES'
    ]
    
    # 알려진 해외 ETF 목록
    KNOWN_FOREIGN_ETFS = {
        'SPY', 'QQQ', 'IWM', 'DIA', 'VTI', 'VOO',
        'TLT', 'GLD', 'SLV', 'USO', 'XLE', 'XLF',
        'JEPI', 'JEPQ', 'SCHD', 'VIG', 'VYM'
    }
    
    def classify_foreign(self, ticker: str, stock_name: str) -> str:
        """해외 종목 분류"""
        # 1. 알려진 ETF 체크
        if ticker.upper() in self.KNOWN_FOREIGN_ETFS:
            return "ETF"
        
        # 2. 이름 패턴 체크
        name_upper = stock_name.upper()
        for pattern in self.FOREIGN_ETF_PATTERNS:
            if pattern in name_upper:
                return "ETF"
        
        # 3. 기본값은 주식
        return "주식"
```

### 3.5 DataValidator 수정

#### 3.5.1 해외 거래 검증
```python
class DataValidator:
    def validate_foreign_trade(self, trade: ForeignTrade) -> Tuple[bool, Optional[str]]:
        """해외 거래 검증"""
        # 통화 검증
        if trade.currency not in ['USD', 'EUR', 'JPY', 'CNY']:
            return False, f"지원하지 않는 통화: {trade.currency}"
        
        # 티커 형식 검증
        if not re.match(r'^[A-Z0-9\-\.]{1,10}$', trade.ticker):
            return False, f"잘못된 티커 형식: {trade.ticker}"
        
        # 환율 검증
        if trade.buy_exchange_rate <= 0 or trade.sell_exchange_rate <= 0:
            return False, "환율은 0보다 커야 합니다"
        
        # 매수/매도 중 하나는 있어야 함
        if trade.buy_quantity == 0 and trade.sell_quantity == 0:
            return False, "매수 또는 매도 수량이 있어야 합니다"
        
        return True, None
```

## 4. 처리 흐름

### 4.1 전체 처리 흐름
```python
# main.py 수정
async def process_trading_log(self, trading_log: TradingLog) -> Dict[str, int]:
    # 1. 파일 타입 확인
    file_type = trading_log.file_type  # "domestic" or "foreign"
    
    # 2. 데이터 검증 (타입별 검증)
    if file_type == "foreign":
        valid_trades, invalid_trades = self.data_validator.validate_foreign_all(trading_log.trades)
    else:
        valid_trades, invalid_trades = self.data_validator.validate_all(trading_log.trades)
    
    # 3. 종목 타입 분류 (타입별 분류)
    if file_type == "foreign":
        stock_types = self.stock_classifier.classify_foreign_batch(valid_trades)
    else:
        stock_types = self.stock_classifier.classify(unique_stocks)
    
    # 4. 대상 시트 찾기 (타입별 시트)
    target_sheets = await self.sheet_manager.find_target_sheets(
        trading_log.prefix, 
        file_type=file_type
    )
    
    # 이후 처리는 동일...
```

## 5. 설정 파일 구조

### 5.1 config.yaml 수정
```yaml
# 시트 이름 패턴
sheet_patterns:
  domestic:
    stock_buy: "{prefix} 주식 매수내역"
    stock_sell: "{prefix} 주식 매도내역"
    etf_buy: "{prefix} ETF 매수내역"
    etf_sell: "{prefix} ETF 매도내역"
  foreign:
    stock_buy: "{prefix} 해외 주식 매수내역"
    stock_sell: "{prefix} 해외 주식 매도내역"
    etf_buy: "{prefix} 해외 ETF 매수내역"
    etf_sell: "{prefix} 해외 ETF 매도내역"

# 지원 통화
supported_currencies:
  - USD
  - EUR
  - JPY
  - CNY

# 해외 ETF 목록 (캐시용)
foreign_etf_cache_file: "foreign_etf_cache.json"
```

## 6. 마이그레이션 계획

### 6.1 단계별 구현
1. **Phase 1**: 기본 구조 변경
   - BaseTrade, DomesticTrade, ForeignTrade 클래스 생성
   - 기존 Trade를 DomesticTrade로 마이그레이션

2. **Phase 2**: 파서 확장
   - 파일 타입 감지 로직 추가
   - 해외 주식 파싱 로직 구현

3. **Phase 3**: 검증 및 분류
   - 해외 주식 검증 로직 추가
   - 해외 ETF 분류 로직 구현

4. **Phase 4**: 시트 관리
   - 해외 주식용 시트 패턴 추가
   - 컬럼 매핑 처리

5. **Phase 5**: 테스트 및 최적화
   - 단위 테스트 작성
   - 통합 테스트 수행

## 7. 주의사항

### 7.1 하위 호환성
- 기존 국내 주식 처리는 그대로 유지
- 설정 파일 변경 시 기본값 제공

### 7.2 성능 고려사항
- 환율 정보 캐싱 고려
- 해외 ETF 목록 주기적 업데이트

### 7.3 에러 처리
- 환율 정보 누락 시 처리
- 통화 변환 오류 처리
- 티커 심볼 검증 강화

## 8. 테스트 계획

### 8.1 단위 테스트
- ForeignTrade 클래스 테스트
- 해외 주식 파서 테스트
- 해외 주식 검증 테스트

### 8.2 통합 테스트
- 국내/해외 혼합 처리 테스트
- 시트 자동 감지 테스트
- 에러 시나리오 테스트 