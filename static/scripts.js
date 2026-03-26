console.log("Script loaded successfully");

function goToPage(url) {
    console.log("Redirecting to:", url); // This should appear in the console
    window.location.href = url;
}

function goBack() {
    window.location.href = "/";
}

