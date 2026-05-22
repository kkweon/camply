const puppeteer = require("puppeteer");

(async () => {
    const browser = await puppeteer.launch({ headless: "new" });
    const page = await browser.newPage();

    page.on("response", async (response) => {
        const url = response.url();
        if (
            url.includes("api") ||
            url.includes("rdr") ||
            url.includes("json") ||
            url.includes("search")
        ) {
            console.log("URL:", url);
        }
    });

    await page.goto("https://www.reservecalifornia.com/park/657/507", {
        waitUntil: "networkidle2",
    });
    await browser.close();
})();
