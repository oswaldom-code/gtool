package launcher.util;

import static org.junit.jupiter.api.Assertions.assertEquals;

import java.util.Map;

import org.junit.jupiter.api.Test;

class RunPortsTest {

    @Test
    void envNameFollowsGtoolConvention() {
        assertEquals("GTOOL_KAFKA_PORT", RunPorts.envName("kafka"));
        assertEquals("GTOOL_KAFKA_DOCKER_PORT", RunPorts.envName("kafka-docker"));
    }

    @Test
    void resolveReturnsInjectedValue() {
        assertEquals("41001", RunPorts.resolve(Map.of("GTOOL_KAFKA_PORT", " 41001 "), "GTOOL_KAFKA_PORT", "9092"));
    }

    @Test
    void resolveFallsBackWhenMissingOrBlank() {
        assertEquals("9092", RunPorts.resolve(Map.of(), "GTOOL_KAFKA_PORT", "9092"));
        assertEquals("9092", RunPorts.resolve(Map.of("GTOOL_KAFKA_PORT", " "), "GTOOL_KAFKA_PORT", "9092"));
    }

    @Test
    void publicAccessorsFallBackOutsideGtool() {
        // The unit-test JVM runs without gtool's variables.
        assertEquals("65000", RunPorts.port("not-a-service", "65000"));
        assertEquals(System.getenv().getOrDefault("GTOOL_APP_URL", "http://localhost:8080"), RunPorts.appUrl());
    }

    @Test
    void settingFallsBackOutsideGtool() {
        assertEquals("postgres", RunPorts.setting("not-a-service", "user", "postgres"));
    }
}
