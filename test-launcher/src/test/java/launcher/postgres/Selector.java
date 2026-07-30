package launcher.postgres;

import java.util.List;
import java.util.Map;

public interface Selector {

    /** Get a single row from a table applying a filter map. */
    Map<String, Object> getRow(String table, Map<String, Object> filter);

    /** Get all rows from a table applying a filter map. */
    List<Map<String, Object>> getRows(String table, Map<String, Object> filter);

    /** Get all rows from a raw SQL query. Use as a last resort. */
    List<Map<String, Object>> getRows(String query);

    /** Get a single column value of the first row matching a filter. */
    Object getField(String table, String field, Map<String, Object> filter);

    /** Check whether at least one row satisfies the filter. */
    Boolean elementExists(String table, Map<String, Object> filter);

    /** Execute a parametrized query (? placeholders) with the given values. */
    List<Map<String, Object>> executeParamQuery(String query, List<Object> values);
}
