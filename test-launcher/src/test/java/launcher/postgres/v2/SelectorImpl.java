package launcher.postgres.v2;

import java.util.HashMap;
import java.util.List;
import java.util.Map;

import launcher.postgres.Selector;

public class SelectorImpl extends DBConnector implements Selector {

    public SelectorImpl(final Map<String, Object> config) {
        super(config);
    }

    @Override
    public Map<String, Object> getRow(final String table, final Map<String, Object> filter) {
        final List<Map<String, Object>> rows = getRows(table, filter);
        return rows.isEmpty() ? new HashMap<>() : rows.get(0);
    }

    @Override
    public List<Map<String, Object>> getRows(final String table, final Map<String, Object> filter) {
        final StringBuilder sb = new StringBuilder("SELECT * FROM ").append(table);
        final List<Object> values = Utils.parametrizeQuery(sb, filter);
        return executeParamQuery(sb.toString(), values);
    }

    @Override
    public List<Map<String, Object>> getRows(final String query) {
        return executeQuery(query);
    }

    @Override
    public Object getField(final String table, final String field, final Map<String, Object> filter) {
        final StringBuilder sb = new StringBuilder("SELECT ").append(field).append(" FROM ").append(table);
        final List<Object> values = Utils.parametrizeQuery(sb, filter);
        sb.append(" LIMIT 1");
        final List<Map<String, Object>> results = executeParamQuery(sb.toString(), values);
        return results.isEmpty() ? null : results.get(0).get(field);
    }

    @Override
    public Boolean elementExists(final String table, final Map<String, Object> filter) {
        final StringBuilder sb = new StringBuilder("SELECT count(1) as total_count FROM ").append(table);
        final List<Object> values = Utils.parametrizeQuery(sb, filter);
        final List<Map<String, Object>> result = executeParamQuery(sb.toString(), values);
        if (!result.isEmpty()) {
            final Object count = result.get(0).get("total_count");
            if (count instanceof Number) {
                return ((Number) count).intValue() > 0;
            }
        }
        return false;
    }
}
