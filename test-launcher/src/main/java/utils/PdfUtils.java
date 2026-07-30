package utils;

import java.awt.Color;
import java.awt.image.BufferedImage;
import java.io.IOException;

import org.apache.pdfbox.pdmodel.PDDocument;
import org.apache.pdfbox.rendering.ImageType;
import org.apache.pdfbox.rendering.PDFRenderer;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * Utility class to compare PDF files page by page, exposed to Karate as {@code pdfu}.
 * On mismatch, a highlighted diff image is written to the Karate reports directory.
 */
public class PdfUtils {

    private final Logger logger = LoggerFactory.getLogger(PdfUtils.class);

    private static final String DIFF_IMAGE = "/app/target/karate-reports/invoice_diff.png";

    public Boolean comparePdfs(final byte[] file1, final byte[] file2) throws IOException {
        try (PDDocument doc1 = PDDocument.load(file1);
                PDDocument doc2 = PDDocument.load(file2)) {

            if (doc1.getNumberOfPages() != doc2.getNumberOfPages()) {
                logger.warn("files page counts do not match - returning false");
                return false;
            }

            final PDFRenderer renderer1 = new PDFRenderer(doc1);
            final PDFRenderer renderer2 = new PDFRenderer(doc2);

            for (int page = 0; page < doc1.getNumberOfPages(); page++) {
                final BufferedImage image1 = renderer1.renderImageWithDPI(page, 300, ImageType.RGB);
                final BufferedImage image2 = renderer2.renderImageWithDPI(page, 300, ImageType.RGB);
                final boolean equal = ImageUtil.compareAndHighlight(image1, image2, DIFF_IMAGE, true,
                        Color.MAGENTA.getRGB());
                if (!equal) {
                    return false;
                }
            }
            return true;
        }
    }
}
