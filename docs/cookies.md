# **📜 Using Cookies for Authentication**

### **Method: Netscape HTTP Cookie File**
To authenticate requests using browser cookies, follow these steps:

> ⚠️ **Important Note:**
> - Always use a **secondary account** for generating cookies.
> - Once cookies are uploaded, **do not log in again** on that account—it may invalidate the session prematurely.

---

## **Step 1: Export Cookies in Netscape Format**
Use a browser extension to export cookies as a **`cookies.txt`** file in **Netscape HTTP format**:

### **Recommended Extensions:**
| Browser     | Extension         | Download Link                                                                                                      |  
|-------------|-------------------|--------------------------------------------------------------------------------------------------------------------|  
| **Chrome**  | `Get cookies.txt` | [Chrome Web Store](https://chromewebstore.google.com/detail/get-cookiestxt-clean/ahmnmhfbokciafffnknlekllgcnafnie) |  
| **Firefox** | `cookies.txt`     | [Firefox Add-ons](https://addons.mozilla.org/en-US/firefox/addon/cookies-txt/)                                     |  

### **How to Export:**
1. Install the extension.
2. Navigate to the target website (YouTube.com) and log in.
3. Click the extension icon and select **"Export cookies.txt"**.
4. Save the file.

---

## **Step 2: Upload Cookies to a Paste Service**
Host your `cookies.txt` on a text-sharing service:

### **🔗 Recommended Paste Services:**
- **[BatBin](https://batbin.me)** (Recommended, no login required)
- **[PasteBin](https://pastebin.com)** (Requires account for long-term pastes)

### **Upload Steps:**
1. Open the paste service.
2. Copy-paste the **entire content** of `cookies.txt`.
3. Click **"Create Paste"** and copy the URL.

---

## **Step 3: Set the Environment Variable**
Add the paste URL to your **`COOKIES_URL`** environment variable.

### **Example:**
```env
COOKIES_URL=https://batbin.me/abc123, https://pastebin.com/xyz456
```  
*(Supports multiple URLs separated by commas)*

---

### **Troubleshooting**
🔸 **Session Invalid?** → Generate new cookies and avoid logging in elsewhere.  
🔸 **403 Errors?** → Ensure cookies are fresh and not expired.

---

### **Best Practices**
✔ **Rotate cookies** periodically to avoid bans.  
✔ **Use private/incognito mode** when generating cookies.  
✔ **Monitor session activity** to detect early invalidation.

---