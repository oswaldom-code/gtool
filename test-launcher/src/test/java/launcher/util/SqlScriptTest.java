package launcher.util;

import static org.junit.jupiter.api.Assertions.assertEquals;

import java.util.List;

import org.junit.jupiter.api.Test;

class SqlScriptTest {

    @Test
    void splitsOnTopLevelSemicolons() {
        assertEquals(List.of("CREATE TABLE a (id int)", "INSERT INTO a VALUES (1)"),
                SqlScript.split("CREATE TABLE a (id int);\nINSERT INTO a VALUES (1);\n", true));
    }

    @Test
    void keepsSemicolonsInsideQuotesAndComments() {
        final String sql = "INSERT INTO t VALUES ('a;b', \"c;d\");\n-- note; not a split\nSELECT 1 /* x; y */;";
        assertEquals(List.of("INSERT INTO t VALUES ('a;b', \"c;d\")", "-- note; not a split\nSELECT 1 /* x; y */"),
                SqlScript.split(sql, true));
    }

    @Test
    void keepsDollarQuotedBodiesWhole() {
        final String fn = "CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END; $$ LANGUAGE plpgsql";
        final String tagged = "CREATE FUNCTION g() RETURNS int AS $body$ SELECT 1; $body$ LANGUAGE sql";
        assertEquals(List.of(fn, tagged), SqlScript.split(fn + ";\n" + tagged + ";", true));
    }

    @Test
    void positionalParametersAreNotDollarQuotes() {
        assertEquals(List.of("PREPARE p AS SELECT $1", "SELECT 2"),
                SqlScript.split("PREPARE p AS SELECT $1;SELECT 2;", true));
    }

    @Test
    void dollarQuotesOnlyWhenEnabled() {
        assertEquals(List.of("SELECT 'x$$'", "SELECT 1"), SqlScript.split("SELECT 'x$$';SELECT 1;", false));
        assertEquals(List.of("SELECT a$$b", "SELECT 2"), SqlScript.split("SELECT a$$b;SELECT 2", false));
    }

    @Test
    void dropsEmptyAndCommentOnlyStatements() {
        assertEquals(List.of("SELECT 1"), SqlScript.split(";;SELECT 1;\n-- trailing comment\n/* end */", true));
    }
}
