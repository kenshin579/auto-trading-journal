package writer

import (
	"reflect"
	"testing"
)

func TestMigrationInserts(t *testing.T) {
	tests := []struct {
		name     string
		old, new []string
		want     []colRun
		wantOK   bool
	}{
		{
			name:   "국내 V2(10) → 신(12): 종목명 뒤 섹터/산업 2열",
			old:    OldDomesticHeadersV2,
			new:    DomesticHeaders,
			want:   []colRun{{start: 4, count: 2}},
			wantOK: true,
		},
		{
			name:   "해외 V1(15) → 신(17): 종목명 뒤 섹터/산업 2열",
			old:    OldForeignHeadersV1,
			new:    ForeignHeaders,
			want:   []colRun{{start: 5, count: 2}},
			wantOK: true,
		},
		{
			name:   "국내 V1(9) → 신(12): 종목코드 1열 + 섹터/산업 2열",
			old:    OldDomesticHeadersV1,
			new:    DomesticHeaders,
			want:   []colRun{{start: 2, count: 1}, {start: 4, count: 2}},
			wantOK: true,
		},
		{
			name:   "이미 신 포맷: 삽입 없음",
			old:    DomesticHeaders,
			new:    DomesticHeaders,
			want:   nil,
			wantOK: true,
		},
		{
			name:   "호환 불가(구 컬럼이 신에 순서대로 없음)",
			old:    []string{"엉뚱", "헤더"},
			new:    DomesticHeaders,
			want:   nil,
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := migrationInserts(tt.old, tt.new)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("runs = %v, want %v", got, tt.want)
			}
		})
	}
}
