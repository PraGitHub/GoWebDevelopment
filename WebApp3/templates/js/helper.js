function onLoadFunction() {
    $("#table-head").hide();
}

function submitSearch() {
    $.ajax({
        url: "/search",
        method: "POST",
        data: $("#search-form").serialize(),
        success: function (rawData) {
            var searchResults = $("#search-results");
            var tableHeading = $("#table-head")
            searchResults.empty();

            var parsed = JSON.parse(rawData);
            if (!parsed) return;

            $("#table-head").show();

            parsed.forEach(function (result) {
                var actionForm = `
                                <form id="${'search-form-' + result.ID}" class="form-inline" onsubmit="return false">
                                    <input class="form-control mr-sm-2" type="text" name="id" value="${result.ID}"  hidden>
                                    <button class="btn btn-outline-success" title="Add this book" type="submit" onclick="return addBook(${result.ID})">
                                        <i class="fas fa-plus-square"></i>
                                    </button>
                                </form>
                            `
                var row = $("<tr><td>" + result.Title + "</td><td>" + result.Author + "</td><td>" + result.Year + "</td><td>" + result.ID + "</td><td>" + actionForm + "</td></tr>");
                searchResults.append(row);
            });
        }
    });
    return false;
}

function addBook(id) {
    $.ajax({
        url: "/books/add",
        method: "POST",
        data: $("#search-form-" + id).serialize(),
        success: function (rawData) {
            var parsed = JSON.parse(rawData);
            if (!parsed) {
                $('#addBookResultModalHead').html(`Sorry <i class="far fa-sad-tear"></i>`);
                $('#addBookResultModalBody').text(`Failed to add the book you selected. Please try again...`);
                $('#addBookResultModal').modal('show');
                return;
            } else if (parsed.BookData) {
                if (parsed.BookData.Title) {
                    $('#addBookResultModalHead').html(`Success <i class="far fa-smile"></i>`);
                    $('#addBookResultModalBody').text(`${parsed.BookData.Title} has been added to database`);
                    $('#addBookResultModal').modal('show');
                } else {
                    $('#addBookResultModalHead').html(`Sorry <i class="far fa-sad-tear"></i>`);
                    $('#addBookResultModalBody').text(`Failed to add the book you selected. It is not popular. Please try again...`);
                    $('#addBookResultModal').modal('show');
                }
            }
        }
    });
    return false;
}