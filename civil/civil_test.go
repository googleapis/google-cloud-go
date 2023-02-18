// Copyright 2016 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package civil

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestDates(t *testing.T) {
	for _, test := range []struct {
		date     Date
		loc      *time.Location
		wantStr  string
		wantTime time.Time
	}{
		{
			date:     Date{2014, 7, 29},
			loc:      time.Local,
			wantStr:  "2014-07-29",
			wantTime: time.Date(2014, time.July, 29, 0, 0, 0, 0, time.Local),
		},
		{
			date:     DateOf(time.Date(2014, 8, 20, 15, 8, 43, 1, time.Local)),
			loc:      time.UTC,
			wantStr:  "2014-08-20",
			wantTime: time.Date(2014, 8, 20, 0, 0, 0, 0, time.UTC),
		},
		{
			date:     DateOf(time.Date(999, time.January, 26, 0, 0, 0, 0, time.Local)),
			loc:      time.UTC,
			wantStr:  "0999-01-26",
			wantTime: time.Date(999, 1, 26, 0, 0, 0, 0, time.UTC),
		},
	} {
		if got := test.date.String(); got != test.wantStr {
			t.Errorf("%#v.String() = %q, want %q", test.date, got, test.wantStr)
		}
		if got := test.date.In(test.loc); !got.Equal(test.wantTime) {
			t.Errorf("%#v.In(%v) = %v, want %v", test.date, test.loc, got, test.wantTime)
		}
	}
}

func TestDateIsValid(t *testing.T) {
	for _, test := range []struct {
		date Date
		want bool
	}{
		{Date{2014, 7, 29}, true},
		{Date{2000, 2, 29}, true},
		{Date{10000, 12, 31}, true},
		{Date{1, 1, 1}, true},
		{Date{0, 1, 1}, true},  // year zero is OK
		{Date{-1, 1, 1}, true}, // negative year is OK
		{Date{1, 0, 1}, false},
		{Date{1, 1, 0}, false},
		{Date{2016, 1, 32}, false},
		{Date{2016, 13, 1}, false},
		{Date{1, -1, 1}, false},
		{Date{1, 1, -1}, false},
	} {
		got := test.date.IsValid()
		if got != test.want {
			t.Errorf("%#v: got %t, want %t", test.date, got, test.want)
		}
	}
}

func TestParseDate(t *testing.T) {
	for _, test := range []struct {
		str  string
		want Date // if empty, expect an error
	}{
		{"2016-01-02", Date{2016, 1, 2}},
		{"2016-12-31", Date{2016, 12, 31}},
		{"0003-02-04", Date{3, 2, 4}},
		{"999-01-26", Date{}},
		{"", Date{}},
		{"2016-01-02x", Date{}},
	} {
		got, err := ParseDate(test.str)
		if got != test.want {
			t.Errorf("ParseDate(%q) = %+v, want %+v", test.str, got, test.want)
		}
		if err != nil && test.want != (Date{}) {
			t.Errorf("Unexpected error %v from ParseDate(%q)", err, test.str)
		}
	}
}

func TestDateArithmetic(t *testing.T) {
	for _, test := range []struct {
		desc  string
		start Date
		end   Date
		days  int
	}{
		{
			desc:  "zero days noop",
			start: Date{2014, 5, 9},
			end:   Date{2014, 5, 9},
			days:  0,
		},
		{
			desc:  "crossing a year boundary",
			start: Date{2014, 12, 31},
			end:   Date{2015, 1, 1},
			days:  1,
		},
		{
			desc:  "negative number of days",
			start: Date{2015, 1, 1},
			end:   Date{2014, 12, 31},
			days:  -1,
		},
		{
			desc:  "full leap year",
			start: Date{2004, 1, 1},
			end:   Date{2005, 1, 1},
			days:  366,
		},
		{
			desc:  "full non-leap year",
			start: Date{2001, 1, 1},
			end:   Date{2002, 1, 1},
			days:  365,
		},
		{
			desc:  "crossing a leap second",
			start: Date{1972, 6, 30},
			end:   Date{1972, 7, 1},
			days:  1,
		},
		{
			desc:  "dates before the unix epoch",
			start: Date{101, 1, 1},
			end:   Date{102, 1, 1},
			days:  365,
		},
	} {
		if got := test.start.AddDays(test.days); got != test.end {
			t.Errorf("[%s] %#v.AddDays(%v) = %#v, want %#v", test.desc, test.start, test.days, got, test.end)
		}
		if got := test.end.DaysSince(test.start); got != test.days {
			t.Errorf("[%s] %#v.Sub(%#v) = %v, want %v", test.desc, test.end, test.start, got, test.days)
		}
	}
}

// Several ways of getting from Fri Nov 18 2011 to Thu Mar 19 2016
var addDateTests = []struct {
	years, months, days int
}{
	{4, 4, 1},
	{3, 16, 1},
	{3, 15, 30},
	{5, -6, -18 - 30 - 12},
}

func TestAddDate(t *testing.T) {
	d0 := Date{2011, 11, 18}
	d1 := Date{2016, 3, 19}
	for _, at := range addDateTests {
		date := d0.AddDate(at.years, at.months, at.days)
		if date != d1 {
			t.Errorf("AddDate(%d, %d, %d) = %v, want %v",
				at.years, at.months, at.days,
				date, d1)
		}
	}
}

func TestDateBefore(t *testing.T) {
	for _, test := range []struct {
		d1, d2 Date
		want   bool
	}{
		{Date{2016, 12, 31}, Date{2017, 1, 1}, true},
		{Date{2016, 1, 1}, Date{2016, 1, 1}, false},
		{Date{2016, 12, 30}, Date{2016, 12, 31}, true},
	} {
		if got := test.d1.Before(test.d2); got != test.want {
			t.Errorf("%v.Before(%v): got %t, want %t", test.d1, test.d2, got, test.want)
		}
	}
}

func TestDateAfter(t *testing.T) {
	for _, test := range []struct {
		d1, d2 Date
		want   bool
	}{
		{Date{2016, 12, 31}, Date{2017, 1, 1}, false},
		{Date{2016, 1, 1}, Date{2016, 1, 1}, false},
		{Date{2016, 12, 30}, Date{2016, 12, 31}, false},
	} {
		if got := test.d1.After(test.d2); got != test.want {
			t.Errorf("%v.After(%v): got %t, want %t", test.d1, test.d2, got, test.want)
		}
	}
}

func TestDateIsZero(t *testing.T) {
	for _, test := range []struct {
		date Date
		want bool
	}{
		{Date{2000, 2, 29}, false},
		{Date{10000, 12, 31}, false},
		{Date{-1, 0, 0}, false},
		{Date{0, 0, 0}, true},
		{Date{}, true},
	} {
		got := test.date.IsZero()
		if got != test.want {
			t.Errorf("%#v: got %t, want %t", test.date, got, test.want)
		}
	}
}

type ISOWeekTest struct {
	year       int // year
	month, day int // month and day
	yex        int // expected year
	wex        int // expected week
}

var isoWeekTests = []ISOWeekTest{
	{1981, 1, 1, 1981, 1}, {1982, 1, 1, 1981, 53}, {1983, 1, 1, 1982, 52},
	{1984, 1, 1, 1983, 52}, {1985, 1, 1, 1985, 1}, {1986, 1, 1, 1986, 1},
	{1987, 1, 1, 1987, 1}, {1988, 1, 1, 1987, 53}, {1989, 1, 1, 1988, 52},
	{1990, 1, 1, 1990, 1}, {1991, 1, 1, 1991, 1}, {1992, 1, 1, 1992, 1},
	{1993, 1, 1, 1992, 53}, {1994, 1, 1, 1993, 52}, {1995, 1, 2, 1995, 1},
	{1996, 1, 1, 1996, 1}, {1996, 1, 7, 1996, 1}, {1996, 1, 8, 1996, 2},
	{1997, 1, 1, 1997, 1}, {1998, 1, 1, 1998, 1}, {1999, 1, 1, 1998, 53},
	{2000, 1, 1, 1999, 52}, {2001, 1, 1, 2001, 1}, {2002, 1, 1, 2002, 1},
	{2003, 1, 1, 2003, 1}, {2004, 1, 1, 2004, 1}, {2005, 1, 1, 2004, 53},
	{2006, 1, 1, 2005, 52}, {2007, 1, 1, 2007, 1}, {2008, 1, 1, 2008, 1},
	{2009, 1, 1, 2009, 1}, {2010, 1, 1, 2009, 53}, {2010, 1, 1, 2009, 53},
	{2011, 1, 1, 2010, 52}, {2011, 1, 2, 2010, 52}, {2011, 1, 3, 2011, 1},
	{2011, 1, 4, 2011, 1}, {2011, 1, 5, 2011, 1}, {2011, 1, 6, 2011, 1},
	{2011, 1, 7, 2011, 1}, {2011, 1, 8, 2011, 1}, {2011, 1, 9, 2011, 1},
	{2011, 1, 10, 2011, 2}, {2011, 1, 11, 2011, 2}, {2011, 6, 12, 2011, 23},
	{2011, 6, 13, 2011, 24}, {2011, 12, 25, 2011, 51}, {2011, 12, 26, 2011, 52},
	{2011, 12, 27, 2011, 52}, {2011, 12, 28, 2011, 52}, {2011, 12, 29, 2011, 52},
	{2011, 12, 30, 2011, 52}, {2011, 12, 31, 2011, 52}, {1995, 1, 1, 1994, 52},
	{2012, 1, 1, 2011, 52}, {2012, 1, 2, 2012, 1}, {2012, 1, 8, 2012, 1},
	{2012, 1, 9, 2012, 2}, {2012, 12, 23, 2012, 51}, {2012, 12, 24, 2012, 52},
	{2012, 12, 30, 2012, 52}, {2012, 12, 31, 2013, 1}, {2013, 1, 1, 2013, 1},
	{2013, 1, 6, 2013, 1}, {2013, 1, 7, 2013, 2}, {2013, 12, 22, 2013, 51},
	{2013, 12, 23, 2013, 52}, {2013, 12, 29, 2013, 52}, {2013, 12, 30, 2014, 1},
	{2014, 1, 1, 2014, 1}, {2014, 1, 5, 2014, 1}, {2014, 1, 6, 2014, 2},
	{2015, 1, 1, 2015, 1}, {2016, 1, 1, 2015, 53}, {2017, 1, 1, 2016, 52},
	{2018, 1, 1, 2018, 1}, {2019, 1, 1, 2019, 1}, {2020, 1, 1, 2020, 1},
	{2021, 1, 1, 2020, 53}, {2022, 1, 1, 2021, 52}, {2023, 1, 1, 2022, 52},
	{2024, 1, 1, 2024, 1}, {2025, 1, 1, 2025, 1}, {2026, 1, 1, 2026, 1},
	{2027, 1, 1, 2026, 53}, {2028, 1, 1, 2027, 52}, {2029, 1, 1, 2029, 1},
	{2030, 1, 1, 2030, 1}, {2031, 1, 1, 2031, 1}, {2032, 1, 1, 2032, 1},
	{2033, 1, 1, 2032, 53}, {2034, 1, 1, 2033, 52}, {2035, 1, 1, 2035, 1},
	{2036, 1, 1, 2036, 1}, {2037, 1, 1, 2037, 1}, {2038, 1, 1, 2037, 53},
	{2039, 1, 1, 2038, 52}, {2040, 1, 1, 2039, 52},
}

func TestISOWeek(t *testing.T) {
	// Selected dates and corner cases
	for _, wt := range isoWeekTests {
		dt := Date{wt.year, time.Month(wt.month), wt.day}
		y, w := dt.ISOWeek()
		if w != wt.wex || y != wt.yex {
			t.Errorf("got %d/%d; expected %d/%d for %d-%02d-%02d",
				y, w, wt.yex, wt.wex, wt.year, wt.month, wt.day)
		}
	}

	// The only real invariant: Jan 04 is in week 1
	for year := 1950; year < 2100; year++ {
		if y, w := (Date{year, time.January, 4}.ISOWeek()); y != year || w != 1 {
			t.Errorf("got %d/%d; expected %d/1 for Jan 04", y, w, year)
		}
	}
}

type YearDayTest struct {
	year, month, day int
	yday             int
}

// Test YearDay in several different scenarios and corner cases
var yearDayTests = []YearDayTest{
	// Non-leap-year tests
	{2007, 1, 1, 1},
	{2007, 1, 15, 15},
	{2007, 2, 1, 32},
	{2007, 2, 15, 46},
	{2007, 3, 1, 60},
	{2007, 3, 15, 74},
	{2007, 4, 1, 91},
	{2007, 12, 31, 365},

	// Leap-year tests
	{2008, 1, 1, 1},
	{2008, 1, 15, 15},
	{2008, 2, 1, 32},
	{2008, 2, 15, 46},
	{2008, 3, 1, 61},
	{2008, 3, 15, 75},
	{2008, 4, 1, 92},
	{2008, 12, 31, 366},

	// Looks like leap-year (but isn't) tests
	{1900, 1, 1, 1},
	{1900, 1, 15, 15},
	{1900, 2, 1, 32},
	{1900, 2, 15, 46},
	{1900, 3, 1, 60},
	{1900, 3, 15, 74},
	{1900, 4, 1, 91},
	{1900, 12, 31, 365},

	// Year one tests (non-leap)
	{1, 1, 1, 1},
	{1, 1, 15, 15},
	{1, 2, 1, 32},
	{1, 2, 15, 46},
	{1, 3, 1, 60},
	{1, 3, 15, 74},
	{1, 4, 1, 91},
	{1, 12, 31, 365},

	// Year minus one tests (non-leap)
	{-1, 1, 1, 1},
	{-1, 1, 15, 15},
	{-1, 2, 1, 32},
	{-1, 2, 15, 46},
	{-1, 3, 1, 60},
	{-1, 3, 15, 74},
	{-1, 4, 1, 91},
	{-1, 12, 31, 365},

	// 400 BC tests (leap-year)
	{-400, 1, 1, 1},
	{-400, 1, 15, 15},
	{-400, 2, 1, 32},
	{-400, 2, 15, 46},
	{-400, 3, 1, 61},
	{-400, 3, 15, 75},
	{-400, 4, 1, 92},
	{-400, 12, 31, 366},

	// Special Cases

	// Gregorian calendar change (no effect)
	{1582, 10, 4, 277},
	{1582, 10, 15, 288},
}

func TestYearDay(t *testing.T) {
	for _, ydt := range yearDayTests {
		dt := Date{ydt.year, time.Month(ydt.month), ydt.day}
		yday := dt.YearDay()
		if yday != ydt.yday {
			t.Errorf("Date(%d-%02d-%02d).YearDay() = %d, want %d",
				ydt.year, ydt.month, ydt.day, yday, ydt.yday)
			continue
		}

		if ydt.year < 0 || ydt.year > 9999 {
			continue
		}
	}
}

func TestTimeToString(t *testing.T) {
	for _, test := range []struct {
		str       string
		time      Time
		roundTrip bool // ParseTime(str).String() == str?
	}{
		{"13:26:33", Time{13, 26, 33, 0}, true},
		{"01:02:03.000023456", Time{1, 2, 3, 23456}, true},
		{"00:00:00.000000001", Time{0, 0, 0, 1}, true},
		{"13:26:03.1", Time{13, 26, 3, 100000000}, false},
		{"13:26:33.0000003", Time{13, 26, 33, 300}, false},
	} {
		gotTime, err := ParseTime(test.str)
		if err != nil {
			t.Errorf("ParseTime(%q): got error: %v", test.str, err)
			continue
		}
		if gotTime != test.time {
			t.Errorf("ParseTime(%q) = %+v, want %+v", test.str, gotTime, test.time)
		}
		if test.roundTrip {
			gotStr := test.time.String()
			if gotStr != test.str {
				t.Errorf("%#v.String() = %q, want %q", test.time, gotStr, test.str)
			}
		}
	}
}

func TestTimeOf(t *testing.T) {
	for _, test := range []struct {
		time time.Time
		want Time
	}{
		{time.Date(2014, 8, 20, 15, 8, 43, 1, time.Local), Time{15, 8, 43, 1}},
		{time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC), Time{0, 0, 0, 0}},
	} {
		if got := TimeOf(test.time); got != test.want {
			t.Errorf("TimeOf(%v) = %+v, want %+v", test.time, got, test.want)
		}
	}
}

func TestTimeIsValid(t *testing.T) {
	for _, test := range []struct {
		time Time
		want bool
	}{
		{Time{0, 0, 0, 0}, true},
		{Time{23, 0, 0, 0}, true},
		{Time{23, 59, 59, 999999999}, true},
		{Time{24, 59, 59, 999999999}, false},
		{Time{23, 60, 59, 999999999}, false},
		{Time{23, 59, 60, 999999999}, false},
		{Time{23, 59, 59, 1000000000}, false},
		{Time{-1, 0, 0, 0}, false},
		{Time{0, -1, 0, 0}, false},
		{Time{0, 0, -1, 0}, false},
		{Time{0, 0, 0, -1}, false},
	} {
		got := test.time.IsValid()
		if got != test.want {
			t.Errorf("%#v: got %t, want %t", test.time, got, test.want)
		}
	}
}

func TestTimeIsZero(t *testing.T) {
	for _, test := range []struct {
		time Time
		want bool
	}{
		{Time{0, 0, 0, 0}, true},
		{Time{}, true},
		{Time{0, 0, 0, 1}, false},
		{Time{-1, 0, 0, 0}, false},
		{Time{0, -1, 0, 0}, false},
	} {
		got := test.time.IsZero()
		if got != test.want {
			t.Errorf("%#v: got %t, want %t", test.time, got, test.want)
		}
	}
}

func TestTimeBefore(t *testing.T) {
	for _, test := range []struct {
		t1, t2 Time
		want   bool
	}{
		{Time{12, 0, 0, 0}, Time{14, 0, 0, 0}, true},
		{Time{12, 20, 0, 0}, Time{12, 30, 0, 0}, true},
		{Time{12, 20, 10, 0}, Time{12, 20, 20, 0}, true},
		{Time{12, 20, 10, 5}, Time{12, 20, 10, 10}, true},
		{Time{12, 20, 10, 5}, Time{12, 20, 10, 5}, false},
	} {
		if got := test.t1.Before(test.t2); got != test.want {
			t.Errorf("%v.Before(%v): got %t, want %t", test.t1, test.t2, got, test.want)
		}
	}
}

func TestTimeAfter(t *testing.T) {
	for _, test := range []struct {
		t1, t2 Time
		want   bool
	}{
		{Time{12, 0, 0, 0}, Time{14, 0, 0, 0}, false},
		{Time{12, 20, 0, 0}, Time{12, 30, 0, 0}, false},
		{Time{12, 20, 10, 0}, Time{12, 20, 20, 0}, false},
		{Time{12, 20, 10, 5}, Time{12, 20, 10, 10}, false},
		{Time{12, 20, 10, 5}, Time{12, 20, 10, 5}, false},
	} {
		if got := test.t1.After(test.t2); got != test.want {
			t.Errorf("%v.After(%v): got %t, want %t", test.t1, test.t2, got, test.want)
		}
	}
}

func TestDateTimeToString(t *testing.T) {
	for _, test := range []struct {
		str       string
		dateTime  DateTime
		roundTrip bool // ParseDateTime(str).String() == str?
	}{
		{"2016-03-22T13:26:33", DateTime{Date{2016, 03, 22}, Time{13, 26, 33, 0}}, true},
		{"2016-03-22T13:26:33.000000600", DateTime{Date{2016, 03, 22}, Time{13, 26, 33, 600}}, true},
		{"2016-03-22t13:26:33", DateTime{Date{2016, 03, 22}, Time{13, 26, 33, 0}}, false},
	} {
		gotDateTime, err := ParseDateTime(test.str)
		if err != nil {
			t.Errorf("ParseDateTime(%q): got error: %v", test.str, err)
			continue
		}
		if gotDateTime != test.dateTime {
			t.Errorf("ParseDateTime(%q) = %+v, want %+v", test.str, gotDateTime, test.dateTime)
		}
		if test.roundTrip {
			gotStr := test.dateTime.String()
			if gotStr != test.str {
				t.Errorf("%#v.String() = %q, want %q", test.dateTime, gotStr, test.str)
			}
		}
	}
}

func TestParseDateTimeErrors(t *testing.T) {
	for _, str := range []string{
		"",
		"2016-03-22",           // just a date
		"13:26:33",             // just a time
		"2016-03-22 13:26:33",  // wrong separating character
		"2016-03-22T13:26:33x", // extra at end
	} {
		if _, err := ParseDateTime(str); err == nil {
			t.Errorf("ParseDateTime(%q) succeeded, want error", str)
		}
	}
}

func TestDateTimeOf(t *testing.T) {
	for _, test := range []struct {
		time time.Time
		want DateTime
	}{
		{time.Date(2014, 8, 20, 15, 8, 43, 1, time.Local),
			DateTime{Date{2014, 8, 20}, Time{15, 8, 43, 1}}},
		{time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC),
			DateTime{Date{1, 1, 1}, Time{0, 0, 0, 0}}},
	} {
		if got := DateTimeOf(test.time); got != test.want {
			t.Errorf("DateTimeOf(%v) = %+v, want %+v", test.time, got, test.want)
		}
	}
}

func TestDateTimeIsValid(t *testing.T) {
	// No need to be exhaustive here; it's just Date.IsValid && Time.IsValid.
	for _, test := range []struct {
		dt   DateTime
		want bool
	}{
		{DateTime{Date{2016, 3, 20}, Time{0, 0, 0, 0}}, true},
		{DateTime{Date{2016, -3, 20}, Time{0, 0, 0, 0}}, false},
		{DateTime{Date{2016, 3, 20}, Time{24, 0, 0, 0}}, false},
	} {
		got := test.dt.IsValid()
		if got != test.want {
			t.Errorf("%#v: got %t, want %t", test.dt, got, test.want)
		}
	}
}

func TestDateTimeIn(t *testing.T) {
	dt := DateTime{Date{2016, 1, 2}, Time{3, 4, 5, 6}}
	got := dt.In(time.UTC)
	want := time.Date(2016, 1, 2, 3, 4, 5, 6, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDateTimeBefore(t *testing.T) {
	d1 := Date{2016, 12, 31}
	d2 := Date{2017, 1, 1}
	t1 := Time{5, 6, 7, 8}
	t2 := Time{5, 6, 7, 9}
	for _, test := range []struct {
		dt1, dt2 DateTime
		want     bool
	}{
		{DateTime{d1, t1}, DateTime{d2, t1}, true},
		{DateTime{d1, t1}, DateTime{d1, t2}, true},
		{DateTime{d2, t1}, DateTime{d1, t1}, false},
		{DateTime{d2, t1}, DateTime{d2, t1}, false},
	} {
		if got := test.dt1.Before(test.dt2); got != test.want {
			t.Errorf("%v.Before(%v): got %t, want %t", test.dt1, test.dt2, got, test.want)
		}
	}
}

func TestDateTimeAfter(t *testing.T) {
	d1 := Date{2016, 12, 31}
	d2 := Date{2017, 1, 1}
	t1 := Time{5, 6, 7, 8}
	t2 := Time{5, 6, 7, 9}
	for _, test := range []struct {
		dt1, dt2 DateTime
		want     bool
	}{
		{DateTime{d1, t1}, DateTime{d2, t1}, false},
		{DateTime{d1, t1}, DateTime{d1, t2}, false},
		{DateTime{d2, t1}, DateTime{d1, t1}, true},
		{DateTime{d2, t1}, DateTime{d2, t1}, false},
	} {
		if got := test.dt1.After(test.dt2); got != test.want {
			t.Errorf("%v.After(%v): got %t, want %t", test.dt1, test.dt2, got, test.want)
		}
	}
}

func TestDateTimeIsZero(t *testing.T) {
	for _, test := range []struct {
		dt   DateTime
		want bool
	}{
		{DateTime{Date{2016, 3, 20}, Time{0, 0, 0, 0}}, false},
		{DateTime{Date{}, Time{5, 44, 20, 0}}, false},
		{DateTime{Date{2016, 3, 20}, Time{}}, false},
		{DateTime{Date{0, 0, 0}, Time{5, 16, 47, 2}}, false},
		{DateTime{Date{2021, 9, 5}, Time{9, 30, 51, 6}}, false},
		{DateTime{Date{}, Time{}}, true},
		{DateTime{Date{0, 0, 0}, Time{0, 0, 0, 0}}, true},
		{DateTime{Date{}, Time{0, 0, 0, 0}}, true},
		{DateTime{Date{0, 0, 0}, Time{}}, true},
	} {
		got := test.dt.IsZero()
		if got != test.want {
			t.Errorf("%#v: got %t, want %t", test.dt, got, test.want)
		}
	}
}

func TestMarshalJSON(t *testing.T) {
	for _, test := range []struct {
		value interface{}
		want  string
	}{
		{Date{1987, 4, 15}, `"1987-04-15"`},
		{Time{18, 54, 2, 0}, `"18:54:02"`},
		{DateTime{Date{1987, 4, 15}, Time{18, 54, 2, 0}}, `"1987-04-15T18:54:02"`},
	} {
		bgot, err := json.Marshal(test.value)
		if err != nil {
			t.Fatal(err)
		}
		if got := string(bgot); got != test.want {
			t.Errorf("%#v: got %s, want %s", test.value, got, test.want)
		}
	}
}

func TestUnmarshalJSON(t *testing.T) {
	var d Date
	var tm Time
	var dt DateTime
	for _, test := range []struct {
		data string
		ptr  interface{}
		want interface{}
	}{
		{`"1987-04-15"`, &d, &Date{1987, 4, 15}},
		{`"1987-04-\u0031\u0035"`, &d, &Date{1987, 4, 15}},
		{`"18:54:02"`, &tm, &Time{18, 54, 2, 0}},
		{`"1987-04-15T18:54:02"`, &dt, &DateTime{Date{1987, 4, 15}, Time{18, 54, 2, 0}}},
	} {
		if err := json.Unmarshal([]byte(test.data), test.ptr); err != nil {
			t.Fatalf("%s: %v", test.data, err)
		}
		if !cmp.Equal(test.ptr, test.want) {
			t.Errorf("%s: got %#v, want %#v", test.data, test.ptr, test.want)
		}
	}

	for _, bad := range []string{"", `""`, `"bad"`, `"1987-04-15x"`,
		`19870415`,     // a JSON number
		`11987-04-15x`, // not a JSON string

	} {
		if json.Unmarshal([]byte(bad), &d) == nil {
			t.Errorf("%q, Date: got nil, want error", bad)
		}
		if json.Unmarshal([]byte(bad), &tm) == nil {
			t.Errorf("%q, Time: got nil, want error", bad)
		}
		if json.Unmarshal([]byte(bad), &dt) == nil {
			t.Errorf("%q, DateTime: got nil, want error", bad)
		}
	}
}
