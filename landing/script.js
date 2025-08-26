// Tab functionality
document.addEventListener('DOMContentLoaded', function () {
    // Tab switching
    const tabButtons = document.querySelectorAll('.tab-btn');
    const tabPanes = document.querySelectorAll('.tab-pane');

    tabButtons.forEach(button => {
        button.addEventListener('click', () => {
            const targetTab = button.getAttribute('data-tab');

            // Remove active class from all buttons and panes
            tabButtons.forEach(btn => btn.classList.remove('active'));
            tabPanes.forEach(pane => pane.classList.remove('active'));

            // Add active class to clicked button and corresponding pane
            button.classList.add('active');
            document.getElementById(targetTab).classList.add('active');
        });
    });

    // Copy functionality
    const copyButtons = document.querySelectorAll('.copy-btn');
    copyButtons.forEach(button => {
        button.addEventListener('click', async () => {
            const codeBlock = button.closest('.code-example')?.querySelector('code');
            if (!codeBlock) return;
            const text = codeBlock.textContent || '';
            const originalText = button.textContent;
            try {
                if (navigator.clipboard?.writeText) {
                    await navigator.clipboard.writeText(text);
                } else {
                    const sel = window.getSelection();
                    const range = document.createRange();
                    range.selectNodeContents(codeBlock);
                    sel?.removeAllRanges();
                    sel?.addRange(range);
                    document.execCommand('copy'); // fallback
                    sel?.removeAllRanges();
                }
                button.textContent = 'Copied!';
                document.getElementById('aria-live')?.append?.('Code copied to clipboard.');
            } catch {
                button.textContent = 'Copy failed';
                document.getElementById('aria-live')?.append?.('Copy to clipboard failed.');
            } finally {
                setTimeout(() => (button.textContent = originalText), 2000);
            }
        });
    });

    // Smooth scrolling for anchor links
    document.querySelectorAll('a[href^="#"]').forEach(anchor => {
        anchor.addEventListener('click', function (e) {
            e.preventDefault();
            const target = document.querySelector(this.getAttribute('href'));
            if (target) {
                target.scrollIntoView({
                    behavior: 'smooth',
                    block: 'start'
                });
            }
        });
    });

    // Theme Switch Functionality
    const themeButtons = document.querySelectorAll('.theme-button');
    const body = document.body;
    const localStorageThemeKey = 'theme';

    function applyTheme(themeName) {
        body.classList.remove('dark-theme'); // Always remove first

        if (themeName === 'dark') {
            body.classList.add('dark-theme');
        } else if (themeName === 'system') {
            if (window.matchMedia('(prefers-color-scheme: dark)').matches) {
                body.classList.add('dark-theme');
            }
        }

        // Update active button state
        themeButtons.forEach(button => {
            const isActive = button.dataset.theme === themeName;
            button.classList.toggle('active', isActive);
        });

        localStorage.setItem(localStorageThemeKey, themeName);
    }

    // Initialize theme based on local storage or system preference
    const savedTheme = localStorage.getItem(localStorageThemeKey);
    if (savedTheme) {
        applyTheme(savedTheme);
    } else {
        applyTheme('system'); // Default to system theme if no preference is saved
    }

    // Add event listeners for theme buttons
    themeButtons.forEach(button => {
        button.addEventListener('click', (event) => {
            const theme = event.target.dataset.theme;
            applyTheme(theme);
        });
    });

    // Listen for system theme changes if the current theme is 'system'
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
        const currentTheme = localStorage.getItem(localStorageThemeKey);
        if (currentTheme === 'system') {
            applyTheme('system'); // Re-apply to reflect system change
        }
    });

    // Set dynamic year in footer
    const currentYearElement = document.getElementById('current-year');
    if (currentYearElement) {
        currentYearElement.textContent = new Date().getFullYear();
    }

    // Fetch GitHub stats initially and set up periodic updates
    fetchGitHubStats();
    setupGitHubStatsRefresh();
});

async function fetchGitHubStats() {
    // Show loading state
    const statsElements = document.querySelectorAll('.github-stats .stat-number');
    statsElements.forEach(el => {
        if (el.textContent !== 'Error') {
            el.textContent = '...';
            el.title = '';
        }
    });

    const repoUrl = 'https://api.github.com/repos/capcom6/mariadb-backup-s3';
    const releasesUrl = 'https://api.github.com/repos/capcom6/mariadb-backup-s3/releases/latest';

    try {
        // Fetch repository data (stars, forks)
        const repoResponse = await fetch(repoUrl);
        if (!repoResponse.ok) {
            throw new Error(`GitHub repo API error: ${repoResponse.status}`);
        }
        const repoData = await repoResponse.json();

        // Update stars and forks with proper formatting
        document.getElementById('stars-count').textContent = repoData.stargazers_count?.toLocaleString() || '0';
        document.getElementById('forks-count').textContent = repoData.forks_count?.toLocaleString() || '0';

        // Fetch latest release data (version)
        const releaseResponse = await fetch(releasesUrl);
        if (releaseResponse.ok) {
            const releaseData = await releaseResponse.json();
            document.getElementById('version').textContent = releaseData.tag_name || 'N/A';
        } else if (releaseResponse.status === 404) {
            // Handle case where there are no releases
            console.warn('No releases found for the repository.');
            document.getElementById('version').textContent = 'No releases';
        } else {
            // Handle other API errors
            console.warn(`GitHub releases API error: ${releaseResponse.status}. Could not fetch latest version.`);
            document.getElementById('version').textContent = 'N/A';
        }

    } catch (error) {
        console.error('Failed to fetch GitHub stats:', error);

        // Show user-friendly error messages
        const statsElements = document.querySelectorAll('.github-stats .stat-number');
        statsElements.forEach(el => {
            if (el.id === 'version' && el.textContent !== 'Error') return; // Don't overwrite version if it's already set
            el.textContent = 'Error';
            el.title = 'Failed to fetch GitHub stats. Please try refreshing the page.';
        });
    }
}

function setupGitHubStatsRefresh() {
    // Refresh stats every 5 minutes (300,000 milliseconds)
    setInterval(fetchGitHubStats, 300000);
}

// Add scroll-triggered animations
const observerOptions = {
    threshold: 0.1,
    rootMargin: '0px 0px -50px 0px'
};

const observer = new IntersectionObserver((entries) => {
    entries.forEach(entry => {
        if (entry.isIntersecting) {
            entry.target.style.opacity = '1';
            entry.target.style.transform = 'translateY(0)';
        }
    });
}, observerOptions);

// Observe elements for scroll animations
document.addEventListener('DOMContentLoaded', () => {
    const animateElements = document.querySelectorAll('.feature, .config-category, .community-item');
    animateElements.forEach(el => {
        el.style.opacity = '0';
        el.style.transform = 'translateY(20px)';
        el.style.transition = 'opacity 0.6s ease, transform 0.6s ease';
        observer.observe(el);
    });
});