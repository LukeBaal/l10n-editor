let languages = [];
let strings = null;
let showTranslations = true;

// window.onload = function () {
//     fetchStrings();
// };

const showTranslationsCheckbox = document.getElementById("showTranslations");
showTranslationsCheckbox.addEventListener("change", (e) => {
    showTranslations = e.target.checked;
    renderTable(languages, strings)

    fetch(`/api/showTranslations?show=${showTranslations}`, {
        method: "PUT"
    }).then(() => {
        filterStrings();
    });
});

function fetchStrings(query) {
    fetch(`/api/strings?query=${query || ""}`)
        .then(response => response.json())
        .then(data => {
            languages = data.langs;
            strings = data.strings;
            showTranslations = data.showTranslations;

            showTranslationsCheckbox.checked = showTranslations;

            renderTable(languages, strings);
        });
}

function renderTable(langs, strings) {
    const thead = document.getElementById('strings-head');
    const tbody = document.getElementById('strings-body');

    // Build Header
    let headerHTML = '<tr><th>Key</th>';
    headerHTML += `<th>Values</th>`;
    headerHTML += '<th>Actions</th></tr>';
    thead.innerHTML = headerHTML;

    // Build Body
    tbody.innerHTML = '';
    for (const key in strings) {
        let rowHTML = `<tr><td>${key}</td>`;
        const values = strings[key];

        const displayNames = new Intl.DisplayNames(["en"], {type: "language"});
        langs.forEach(lang => {
            if (!showTranslations && lang !== "en") {
                return;
            }
            const displayName = displayNames.of(lang);
            const value = values[lang] || '';
            const editable = lang == "en";
            rowHTML += `<td class="value-cell">
${showTranslations ? `<label for="value-${key}-${lang}"/>${displayName}</label>` : ""}
<input type="text" id="value-${key}-${lang}" value="${escapeHTML(value)}" title="${escapeHTML(value)}" ${editable ? "" : "disabled"}>
<button class="save" onclick="editString('${key}', '${lang}')">Save</button>
</td>`;
        });

        rowHTML += `<td class="actions"><button class="remove" onclick="removeString('${key}')">Remove Key</button></td>`;
        rowHTML += '</tr>';
        tbody.innerHTML += rowHTML;
    }
}

let filterTimeout = null;

function filterStrings() {
    if (filterTimeout) {
        clearTimeout(filterTimeout);
    }
    filterTimeout = setTimeout(function() {
        const filter = document.getElementById('search').value.toLowerCase();
        fetchStrings(filter);
    }, 300);
    // const table = document.getElementById('strings-table');
    // const tr = table.getElementsByTagName('tr');
    //
    // for (let i = 1; i < tr.length; i++) {
    //     tr[i].style.display = "none"; // Hide by default
    //     const cells = tr[i].getElementsByTagName('td');
    //     let found = false;
    //     // Search key cell (index 0) and all value inputs
    //     if (cells[0].textContent.toLowerCase().indexOf(filter) > -1) {
    //         found = true;
    //     } else {
    //         for (let j = 1; j < cells.length - 1; j++) { // minus 1 for action cell
    //             const input = cells[j].getElementsByTagName('input')[0];
    //             if (input && input.value.toLowerCase().indexOf(filter) > -1) {
    //                 found = true;
    //                 break;
    //             }
    //         }
    //     }
    //     if (found) {
    //         tr[i].style.display = "";
    //     }
    // }
}

function addString() {
    const key = document.getElementById('new-key').value;
    const value = document.getElementById('new-value').value;
    if (!value) {
        alert('English Value cannot be empty.');
        return;
    }

    const formData = new FormData();
    formData.append('key', key);
    formData.append('value', value);

    fetch('/api/add', {method: 'POST', body: formData})
        .then(response => {
            if (response.ok) {
                fetchStrings();
                document.getElementById('new-key').value = '';
                document.getElementById('new-value').value = '';
            } else {
                response.text().then(text => alert('Error adding string: ' + text));
            }
        });
}

function editString(key, lang) {
    const value = document.getElementById(`value-${key}-${lang}`).value;
    const formData = new FormData();
    formData.append('key', key);
    formData.append('lang', lang);
    formData.append('value', value);

    fetch('/api/edit', {method: 'POST', body: formData})
        .then(response => {
            if (response.ok) {
                alert(`'${key}' in '${lang}' saved!`);
            } else {
                alert('Error saving string.');
            }
        });
}

function removeString(key) {
    if (confirm(`Are you sure you want to remove the key '${key}' from all language files?`)) {
        const formData = new FormData();
        formData.append('key', key);

        fetch('/api/remove', {method: 'POST', body: formData})
            .then(response => {
                if (response.ok) {
                    fetchStrings();
                } else {
                    alert('Error removing string.');
                }
            });
    }
}

function escapeHTML(str) {
    const p = document.createElement('p');
    p.textContent = str;
    return p.innerHTML;
}
