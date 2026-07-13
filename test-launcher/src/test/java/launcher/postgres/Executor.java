package launcher.postgres;

import java.sql.SQLException;

public interface Executor {

    /**
     * Execute a SQL script (statements separated by {@code ;}).
     *
     * @param scriptName optional name used in trace logs
     * @param sql        the SQL script
     * @return true if the script executed without errors
     */
    Boolean executeSqlScript(String scriptName, String sql) throws SQLException;
}
