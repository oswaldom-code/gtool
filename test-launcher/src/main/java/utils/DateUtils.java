package utils;

import java.time.OffsetDateTime;
import java.time.ZoneId;
import java.time.ZonedDateTime;
import java.time.format.DateTimeFormatter;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * Utility class to manipulate dates, exposed to Karate as {@code du}.
 */
public class DateUtils {

    private final Logger logger = LoggerFactory.getLogger(DateUtils.class);

    private static final DateTimeFormatter DATE_TIME_FORMATTER = DateTimeFormatter.ISO_OFFSET_DATE_TIME;

    /**
     * Compares two ISO_OFFSET dates and checks whether they point to the same instant.
     */
    public Boolean compareDates(final String date1, final String date2) {
        final OffsetDateTime odt1 = OffsetDateTime.parse(date1, DATE_TIME_FORMATTER);
        final OffsetDateTime odt2 = OffsetDateTime.parse(date2, DATE_TIME_FORMATTER);
        final boolean ret = odt1.isEqual(odt2);

        logger.debug("Comparing dates: {} with {}. Result: {}", date1, date2, ret);
        return ret;
    }

    /**
     * Compares two ISO_OFFSET dates with a tolerance threshold expressed in seconds.
     */
    public Boolean compareDates(final String date1, final String date2, final long threshold) {
        final OffsetDateTime odt1 = OffsetDateTime.parse(date1, DATE_TIME_FORMATTER);
        final OffsetDateTime odt2 = OffsetDateTime.parse(date2, DATE_TIME_FORMATTER);
        final long diff = odt2.toEpochSecond() - odt1.toEpochSecond();

        return Math.abs(diff) <= threshold;
    }

    /**
     * Checks whether date1 isAfter date2, taking the timezone into account.
     */
    public Boolean isAfter(final String date1, final String date2) {
        final OffsetDateTime odt1 = OffsetDateTime.parse(date1, DATE_TIME_FORMATTER);
        final OffsetDateTime odt2 = OffsetDateTime.parse(date2, DATE_TIME_FORMATTER);

        return odt1.isAfter(odt2);
    }

    /**
     * Checks whether date1 isBefore date2, taking the timezone into account.
     */
    public Boolean isBefore(final String date1, final String date2) {
        final OffsetDateTime odt1 = OffsetDateTime.parse(date1, DATE_TIME_FORMATTER);
        final OffsetDateTime odt2 = OffsetDateTime.parse(date2, DATE_TIME_FORMATTER);

        return odt1.isBefore(odt2);
    }

    /**
     * Returns the given date converted to the system default timezone.
     * Prefer {@link #dateInTimeZone(String, String)} with an explicit zone (e.g. "Europe/Madrid").
     */
    public String dateInCurrentTZ(final String date) {
        return dateInTimeZone(date, ZoneId.systemDefault().normalized().toString());
    }

    /**
     * Returns the given date converted to the given normalized timezone (e.g. "Europe/Madrid").
     */
    public String dateInTimeZone(final String date, final String timeZone) {
        final ZoneId zId = ZoneId.of(timeZone);
        final ZonedDateTime odt = OffsetDateTime.parse(date).atZoneSameInstant(zId);
        final String ret = odt.format(DATE_TIME_FORMATTER);
        logger.debug("date {} in timeZone {}: {}", date, timeZone, ret);

        return ret;
    }
}
