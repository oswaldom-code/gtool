package launcher.postgres.v2;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

public final class Utils {

    private Utils() {
    }

    /**
     * Appends the filter part of a query ({@code WHERE k = ? AND ...}) to the StringBuilder and
     * returns the ordered list of values to bind to the placeholders.
     */
    public static List<Object> parametrizeQuery(final StringBuilder sb, final Map<String, Object> filter) {
        boolean first = true;
        final List<Object> values = new ArrayList<>();
        for (final Map.Entry<String, Object> entry : filter.entrySet()) {
            sb.append(first ? " WHERE " : " AND ");
            first = false;
            sb.append(entry.getKey()).append(" = ? ");
            values.add(entry.getValue());
        }
        return values;
    }
}
