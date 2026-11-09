package launcher.kafka;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.util.Set;

import org.junit.jupiter.api.Test;

class KafkaClientTest {

    @Test
    void matchesEqualMessage() {
        assertTrue(KafkaClient.matches("{\"id\":\"1\",\"qty\":2}", "{\"id\":\"1\",\"qty\":2}", Set.of()));
    }

    @Test
    void matchesIgnoringKeyOrder() {
        assertTrue(KafkaClient.matches("{\"id\":\"1\",\"qty\":2}", "{\"qty\":2,\"id\":\"1\"}", Set.of()));
    }

    @Test
    void doesNotMatchDifferentValue() {
        assertFalse(KafkaClient.matches("{\"id\":\"1\",\"qty\":2}", "{\"id\":\"1\",\"qty\":3}", Set.of()));
    }

    @Test
    void matchesIgnoringFields() {
        assertTrue(KafkaClient.matches("{\"id\":\"1\"}", "{\"id\":\"1\",\"ts\":\"now\"}", Set.of("ts")));
    }

    @Test
    void doesNotMatchInvalidJson() {
        assertFalse(KafkaClient.matches("{\"id\":\"1\"}", "not json", Set.of()));
    }

    @Test
    void consumedOffsetOnlyMovesForward() {
        final org.apache.kafka.common.TopicPartition p = new org.apache.kafka.common.TopicPartition("orders-test", 0);
        KafkaClient.markConsumed(p, 4);
        KafkaClient.markConsumed(p, 2);
        assertEquals(5L, KafkaClient.consumedOffset(p), "resume after the furthest matched message");
    }
}
