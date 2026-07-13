package utils;

import java.awt.image.BufferedImage;
import java.io.File;
import java.io.IOException;
import java.util.Arrays;

import javax.imageio.ImageIO;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

class ImageUtil {

    static Logger logger = LoggerFactory.getLogger(ImageUtil.class);

    static boolean compareAndHighlight(final BufferedImage img1, final BufferedImage img2,
            final String fileName, final boolean highlight, final int colorCode) throws IOException {

        final int w = img1.getWidth();
        final int h = img1.getHeight();
        final int[] p1 = img1.getRGB(0, 0, w, h, null, 0, w);
        final int[] p2 = img2.getRGB(0, 0, w, h, null, 0, w);

        if (!Arrays.equals(p1, p2)) {
            logger.warn("Image compared - does not match");
            if (highlight) {
                for (int i = 0; i < p1.length; i++) {
                    if (p1[i] != p2[i]) {
                        p1[i] = colorCode;
                    }
                }
                final BufferedImage out = new BufferedImage(w, h, BufferedImage.TYPE_INT_ARGB);
                out.setRGB(0, 0, w, h, p1, 0, w);
                saveImage(out, fileName);
            }
            return false;
        }
        return true;
    }

    static void saveImage(final BufferedImage image, final String file) {
        try {
            final File outputFile = new File(file);
            ImageIO.write(image, "png", outputFile);
        } catch (Exception e) {
            logger.error("Could not save image {}", file, e);
        }
    }
}
