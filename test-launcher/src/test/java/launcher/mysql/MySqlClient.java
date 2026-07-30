package launcher.mysql;

import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.ResultSetMetaData;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import launcher.postgres.v2.Utils;

/**
 * MySQL/MariaDB helper for seeding and asserting state against a running database.
 * Mirrors {@code PostgresClient}'s API (map filters → WHERE clauses) over a JDBC
 * connection of its own. Exposed to Karate via Java.type.
 */
public class MySqlClient {

    private static final Logger logger = LoggerFactory.getLogger(MySqlClient.class);

    private final String url;
    private final String user;
    private final String password;

    /**
     * @param config {@code url} (jdbc:mysql://...), {@code user}, {@code password}.
     */
    public MySqlClient(final Map<String, Object> config) {
        this.url = (String) config.get("url");
        this.user = (String) config.get("user");
        this.password = (String) config.get("password");
    }

    private Connection open() throws SQLException {
        final Connection con = DriverManager.getConnection(url, user, password);
        con.setAutoCommit(true);
        return con;
    }

    /** Get all rows from a table matching the filter. */
    public List<Map<String, Object>> getRows(final String table, final Map<String, Object> filter) {
        final StringBuilder sb = new StringBuilder("SELECT * FROM ").append(table);
        final List<Object> values = Utils.parametrizeQuery(sb, filter);
        return executeParamQuery(sb.toString(), values);
    }

    /** Get the first row from a table matching the filter. */
    public Map<String, Object> getRow(final String table, final Map<String, Object> filter) {
        final List<Map<String, Object>> rows = getRows(table, filter);
        return rows.isEmpty() ? new HashMap<>() : rows.get(0);
    }

    /** Get a single column value of the first row matching the filter. */
    public Object getField(final String table, final String field, final Map<String, Object> filter) {
        final StringBuilder sb = new StringBuilder("SELECT ").append(field).append(" FROM ").append(table);
        final List<Object> values = Utils.parametrizeQuery(sb, filter);
        sb.append(" LIMIT 1");
        final List<Map<String, Object>> results = executeParamQuery(sb.toString(), values);
        return results.isEmpty() ? null : results.get(0).get(field);
    }

    /** Check whether at least one row satisfies the filter. */
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

    /** Update a column for every row matching the filter. Returns true if any row changed. */
    public Boolean updateField(final String table, final String fieldToUpdate, final Object newValue,
            final Map<String, Object> filter) {
        final StringBuilder sb = new StringBuilder("UPDATE ").append(table)
                .append(" SET ").append(fieldToUpdate).append(" = ? ");
        final List<Object> values = Utils.parametrizeQuery(sb, filter);
        values.add(0, newValue);
        return executeParamUpdate(sb.toString(), values) > 0;
    }

    /** Delete every row matching the filter. Returns true if any row was deleted. */
    public Boolean deleteFromTable(final String table, final Map<String, Object> filter) {
        if (filter.isEmpty()) {
            logger.warn("{deleteFromTable} called without any filter value: not allowed");
            return false;
        }
        final StringBuilder sb = new StringBuilder("DELETE FROM ").append(table).append(" ");
        final List<Object> values = Utils.parametrizeQuery(sb, filter);
        return executeParamUpdate(sb.toString(), values) > 0;
    }

    /** Execute a parametrized query (? placeholders) with the given values. */
    public List<Map<String, Object>> executeParamQuery(final String query, final List<Object> values) {
        final List<Map<String, Object>> resultRows = new ArrayList<>();
        try (Connection con = open(); PreparedStatement pst = con.prepareStatement(query)) {
            for (int x = 0; x < values.size(); x++) {
                pst.setObject(x + 1, values.get(x));
            }
            try (ResultSet rs = pst.executeQuery()) {
                final ResultSetMetaData meta = rs.getMetaData();
                while (rs.next()) {
                    final Map<String, Object> row = new HashMap<>();
                    for (int i = 1; i <= meta.getColumnCount(); i++) {
                        row.put(meta.getColumnLabel(i), rs.getObject(i));
                    }
                    resultRows.add(row);
                }
            }
        } catch (Exception e) {
            logger.error("{executeParamQuery} Error executing query {}", query, e);
        }
        return resultRows;
    }

    /** Execute a SQL script (statements separated by {@code ;}). */
    public Boolean executeSqlScript(final String sql) {
        try (Connection con = open(); Statement st = con.createStatement()) {
            for (final String statement : sql.split(";")) {
                final String query = statement + ";";
                if (!query.trim().equals(";")) {
                    st.executeUpdate(query);
                }
            }
            return true;
        } catch (SQLException e) {
            logger.error("{executeSqlScript} Could not execute script", e);
            return false;
        }
    }

    private int executeParamUpdate(final String query, final List<Object> values) {
        try (Connection con = open(); PreparedStatement stmt = con.prepareStatement(query)) {
            for (int x = 0; x < values.size(); x++) {
                stmt.setObject(x + 1, values.get(x));
            }
            return stmt.executeUpdate();
        } catch (SQLException e) {
            logger.error("{executeParamUpdate} Error executing query {}. Values: {}.", query, values, e);
            return 0;
        }
    }
}
