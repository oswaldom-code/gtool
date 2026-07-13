package launcher.postgres;

import java.util.Map;

public interface Updater {

    /**
     * Update a column for every row matching the filter.
     *
     * @return true if at least one row was updated.
     */
    Boolean updateField(String table, String fieldToUpdate, Object newValue, Map<String, Object> filter);

    /**
     * Delete every row matching the filter.
     *
     * @return true if at least one row was deleted.
     */
    Boolean deleteFromTable(String table, Map<String, Object> filter);
}
