#!/bin/bash

# 색상 정의
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# 가상환경 확인 및 활성화
VENV_DIR=".venv"
if [ ! -d "$VENV_DIR" ]; then
    echo -e "${RED}가상환경을 찾을 수 없습니다: $VENV_DIR${NC}"
    echo -e "${BLUE}가상환경을 먼저 생성해주세요: python3 -m venv $VENV_DIR${NC}"
    exit 1
fi

# 가상환경 활성화
echo -e "${BLUE}가상환경 활성화: $VENV_DIR${NC}"
source "$VENV_DIR/bin/activate"

# Python 버전 확인
echo -e "${BLUE}Python 버전: $(python --version)${NC}"
echo -e "${BLUE}Python 경로: $(which python)${NC}"

# 타임스탬프 생성 (YYYYMMDD_HHMMSS 형식)
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")

# 로그 디렉토리 확인 및 생성
LOG_DIR="logs"
if [ ! -d "$LOG_DIR" ]; then
    echo -e "${BLUE}로그 디렉토리 생성: $LOG_DIR${NC}"
    mkdir -p "$LOG_DIR"
fi

# 로그 파일 경로
LOG_FILE="$LOG_DIR/run_${TIMESTAMP}.log"

# 실행 시작 알림
echo -e "${GREEN}=== 주식 데이터 구글 시트 입력 스크립트 실행 ===${NC}"
echo -e "${BLUE}로그 레벨: DEBUG${NC}"
echo -e "${BLUE}로그 파일: $LOG_FILE${NC}"
echo ""

# CSV 인코딩 변환 (CP949 → UTF-8)
echo -e "${BLUE}=== CSV 인코딩 확인 및 변환 ===${NC}"
CONVERTED=0
SKIPPED=0
while IFS= read -r -d '' csv_file; do
    encoding=$(file -I "$csv_file" | grep -o 'charset=.*' | cut -d= -f2)
    if [[ "$encoding" == "utf-8" ]]; then
        SKIPPED=$((SKIPPED + 1))
    else
        iconv -f CP949 -t UTF-8 "$csv_file" > "$csv_file.tmp" && mv "$csv_file.tmp" "$csv_file"
        echo -e "${GREEN}  변환 완료: $(basename "$csv_file")${NC}"
        CONVERTED=$((CONVERTED + 1))
    fi
done < <(find input -name "*.csv" -print0 2>/dev/null)
echo -e "${BLUE}인코딩 변환: ${CONVERTED}개 변환, ${SKIPPED}개 스킵 (이미 UTF-8)${NC}"
echo ""

# Python 스크립트 실행 (DEBUG 레벨로 설정하고 로그 파일에 저장)
# tee를 사용하여 터미널과 파일 모두에 출력
python main.py --log-level DEBUG 2>&1 | tee "$LOG_FILE"

# 실행 결과 확인
if [ ${PIPESTATUS[0]} -eq 0 ]; then
    echo ""
    echo -e "${GREEN}=== 스크립트 실행 완료 ===${NC}"
    echo -e "${BLUE}로그가 저장되었습니다: $LOG_FILE${NC}"
else
    echo ""
    echo -e "${RED}=== 스크립트 실행 중 오류 발생 ===${NC}"
    echo -e "${BLUE}자세한 내용은 로그를 확인하세요: $LOG_FILE${NC}"
    exit 1
fi

# 가상환경 비활성화
deactivate 