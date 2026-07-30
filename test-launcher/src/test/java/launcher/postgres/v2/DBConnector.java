package launcher.postgres.v2;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.ResultSetMetaData;
import java.sql.SQLException;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import launcher.postgres.DatabaseConnectionSingleton;

/**
 * Base class providing parametrized query/update execution against PostgreSQL.
 */
public abstract class DBConnector {

    protected final Map<String, Object> config;

    private static final Logger logger = LoggerFactory.getLogger(DBConnector.class);

    protected DBConnector(final Map<String, Object> config) {
        this.config = config;
    }

    /**
     * Execute a parametrized query (? placeholders) and return the rows as maps.
     */
    public List<Map<String, Object>> executeParamQuery(final String query, final List<Object> values) {
        final Connection con = DatabaseConnectionSingleton.getInstance(config).connection;
        final List<Map<String, Object>> resultRows = new ArrayList<>();

        try (PreparedStatement pst = con.prepareStatement(query)) {
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

    /**
     * Execute a raw SELECT query with no parameters.
     */
    protected List<Map<String, Object>> executeQuery(final String query) {
        return executeParamQuery(query, new ArrayList<>());
    }

    /**
     * Execute a parametrized UPDATE/DELETE and return the number of affected rows.
     */
    protected int executeParamUpdate(final String query, final List<Object> values) {
        int numRowsAffected = 0;
        final Connection con = DatabaseConnectionSingleton.getInstance(config).connection;
        try (PreparedStatement stmt = con.prepareStatement(query)) {
            for (int x = 0; x < values.size(); x++) {
                stmt.setObject(x + 1, values.get(x));
            }
            numRowsAffected = stmt.executeUpdate();
            if (numRowsAffected == 0) {
                logger.warn("{executeParamUpdate} 0 rows affected. Query: {}. Values: {}.", query, values);
            }
        } catch (SQLException e) {
            logger.error("{executeParamUpdate} Error executing query {}. Values: {}.", query, values, e);
        }
        return numRowsAffected;
    }
}
