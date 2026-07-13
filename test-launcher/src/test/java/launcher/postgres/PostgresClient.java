package launcher.postgres;

import java.sql.SQLException;
import java.util.List;
import java.util.Map;

import launcher.postgres.v2.ExecutorImpl;
import launcher.postgres.v2.SelectorImpl;
import launcher.postgres.v2.UpdaterImpl;

/**
 * Facade for every interaction with a PostgreSQL database, exposed to Karate as {@code ps}.
 *
 * <p>Operations are split across {@link Selector} (reads), {@link Updater} (writes) and
 * {@link Executor} (raw scripts).
 *
 * <p>Filters use a map syntax: {@code { 'column': value }} → {@code WHERE column = value}.
 */
public class PostgresClient {

    private final Selector selector;
    private final Updater updater;
    private final Executor executor;

    /**
     * @param config connection settings: {@code url}, {@code user}, {@code password}.
     */
    public PostgresClient(final Map<String, Object> config) {
        this.selector = new SelectorImpl(config);
        this.updater = new UpdaterImpl(config);
        this.executor = new ExecutorImpl(config);
    }

    /** Get all rows from a table matching the filter. */
    public List<Map<String, Object>> getRows(final String table, final Map<String, Object> filter) {
        return selector.getRows(table, filter);
    }

    /** Get the first row from a table matching the filter. */
    public Map<String, Object> getRow(final String table, final Map<String, Object> filter) {
        return selector.getRow(table, filter);
    }

    /** Get all rows from a raw SQL query (parameters must be escaped). */
    public List<Map<String, Object>> getRows(final String query) {
        return selector.getRows(query);
    }

    /** Get a single column value of the first row matching the filter. */
    public Object getField(final String table, final String field, final Map<String, Object> filter) {
        return selector.getField(table, field, filter);
    }

    /** Check whether at least one row satisfies the filter. */
    public Boolean elementExists(final String table, final Map<String, Object> filter) {
        return selector.elementExists(table, filter);
    }

    /** Update a column for every row matching the filter. Returns true if any row changed. */
    public Boolean updateField(final String table, final String fieldToUpdate, final Object newValue,
            final Map<String, Object> filter) {
        return updater.updateField(table, fieldToUpdate, newValue, filter);
    }

    /** Delete every row matching the filter. Returns true if any row was deleted. */
    public Boolean deleteFromTable(final String table, final Map<String, Object> filter) {
        return updater.deleteFromTable(table, filter);
    }

    /** Execute an anonymous SQL script. */
    public Boolean executeSqlScript(final String sql) throws SQLException {
        return executor.executeSqlScript(null, sql);
    }

    /** Execute a named SQL script (name used in trace logs). */
    public Boolean executeSqlScript(final String scriptName, final String sql) throws SQLException {
        return executor.executeSqlScript(scriptName, sql);
    }

    /** Execute a parametrized query (? placeholders) with the given values. */
    public List<Map<String, Object>> executeParamQuery(final String query, final List<Object> values) {
        return selector.executeParamQuery(query, values);
    }
}
