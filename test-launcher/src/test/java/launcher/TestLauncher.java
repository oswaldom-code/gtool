package launcher;

import static org.junit.jupiter.api.Assertions.assertEquals;

import com.intuit.karate.Results;
import com.intuit.karate.Runner;

import org.junit.jupiter.api.Test;

class TestLauncher {

    @Test
    void testParallel() {
        Results results = Runner.path("classpath:launcher/features").outputCucumberJson(true).parallel(5);
        assertEquals(0, results.getFailCount(), results.getErrorMessages());
    }

}
