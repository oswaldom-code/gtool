package utils;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

import org.junit.jupiter.api.Test;

class DateUtilsTest {

    private final DateUtils dateUtils = new DateUtils();

    @Test
    void compareDatesSameDateSameOffset() {
        assertTrue(dateUtils.compareDates("2022-05-26T07:00:21Z", "2022-05-26T07:00:21Z"));
    }

    @Test
    void compareDatesDifferentOffset() {
        assertTrue(dateUtils.compareDates("2022-05-26T07:00:21Z", "2022-05-26T08:00:21+01:00"));
    }

    @Test
    void dateInCurrentTZ() {
        final String date = "2022-09-22T19:45:06Z";
        assertTrue(dateUtils.compareDates(date, dateUtils.dateInCurrentTZ(date)));
    }

    @Test
    void compareDatesWithThresholdSameOffset() {
        assertTrue(dateUtils.compareDates("2022-05-26T07:00:21Z", "2022-05-26T07:00:25Z", 5L));
    }

    @Test
    void compareDatesWithThresholdDifferentOffset() {
        assertTrue(dateUtils.compareDates("2022-05-26T07:00:21Z", "2022-05-26T08:00:25+01:00", 5L));
    }

    @Test
    void isAfterDifferentOffset() {
        final String date1 = "2022-05-26T07:00:21Z";
        final String date2 = "2022-05-26T08:00:25+01:00";
        assertTrue(dateUtils.isAfter(date2, date1));
        assertFalse(dateUtils.isAfter(date1, date2));
    }

    @Test
    void isBeforeDifferentOffset() {
        final String date1 = "2022-05-26T07:00:21Z";
        final String date2 = "2022-05-26T08:00:25+01:00";
        assertFalse(dateUtils.isBefore(date2, date1));
        assertTrue(dateUtils.isBefore(date1, date2));
    }
}
